<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/core';

    export let user;

    let banners = [];
    let currentIndex = 0;
    let loading = true;

    onMount(async () => {
        try {
            const res = await api.get('/plugins/banner/list?position=header');
            banners = res.banners || [];
            if (banners.length > 1) {
                startAutoSlide();
            }
        } catch (e) {
            console.error('Failed to load header banners:', e);
        } finally {
            loading = false;
        }
    });

    function startAutoSlide() {
        setInterval(() => {
            currentIndex = (currentIndex + 1) % banners.length;
        }, 5000);
    }

    function trackView(bannerId) {
        api.post(`/plugins/banner/${bannerId}/view`).catch(() => {});
    }

    $: if (banners.length > 0) {
        trackView(banners[currentIndex]?.id);
    }
</script>

{#if !loading && banners.length > 0}
    <div class="header-banner">
        <div class="banner-slider" style="transform: translateX(-{currentIndex * 100}%)">
            {#each banners as banner}
                <a
                    href="/api/plugins/banner/{banner.id}/click"
                    target={banner.target || '_blank'}
                    rel="noopener noreferrer"
                    class="banner-slide"
                >
                    {#if banner.image_url}
                        <img
                            src={banner.image_url}
                            alt={banner.alt_text || banner.title}
                            loading="lazy"
                        />
                    {/if}
                </a>
            {/each}
        </div>
        {#if banners.length > 1}
            <div class="banner-dots">
                {#each banners as _, i}
                    <button
                        class="dot"
                        class:active={i === currentIndex}
                        on:click={() => currentIndex = i}
                        aria-label="Banner {i + 1}"
                    ></button>
                {/each}
            </div>
        {/if}
    </div>
{/if}

<style>
    .header-banner {
        position: relative;
        width: 100%;
        overflow: hidden;
        background: #f8f9fa;
    }

    .banner-slider {
        display: flex;
        transition: transform 0.5s ease-in-out;
    }

    .banner-slide {
        flex: 0 0 100%;
        display: block;
    }

    .banner-slide img {
        width: 100%;
        height: auto;
        display: block;
    }

    .banner-dots {
        position: absolute;
        bottom: 12px;
        left: 50%;
        transform: translateX(-50%);
        display: flex;
        gap: 8px;
    }

    .dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        border: none;
        background: rgba(255, 255, 255, 0.5);
        cursor: pointer;
        padding: 0;
        transition: background 0.2s;
    }

    .dot.active {
        background: #fff;
    }

    .dot:hover {
        background: rgba(255, 255, 255, 0.8);
    }
</style>
