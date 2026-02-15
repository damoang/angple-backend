<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/core';

    export let user;

    let banners = [];
    let loading = true;
    let error = null;

    onMount(async () => {
        try {
            const res = await api.get('/plugins/banner/list?position=sidebar');
            banners = res.banners || [];
            // 노출 트래킹
            banners.forEach(banner => {
                api.post(`/plugins/banner/${banner.id}/view`).catch(() => {});
            });
        } catch (e) {
            error = e.message;
        } finally {
            loading = false;
        }
    });
</script>

{#if loading}
    <div class="sidebar-banner loading">
        <div class="skeleton"></div>
    </div>
{:else if error}
    <!-- 에러 시 조용히 숨김 -->
{:else if banners.length > 0}
    <div class="sidebar-banner">
        {#each banners as banner}
            <a
                href="/api/plugins/banner/{banner.id}/click"
                target={banner.target || '_blank'}
                rel="noopener noreferrer"
                class="banner-item"
            >
                {#if banner.image_url}
                    <img
                        src={banner.image_url}
                        alt={banner.alt_text || banner.title}
                        loading="lazy"
                    />
                {:else}
                    <div class="text-banner">
                        {banner.title}
                    </div>
                {/if}
            </a>
        {/each}
    </div>
{/if}

<style>
    .sidebar-banner {
        display: flex;
        flex-direction: column;
        gap: 12px;
    }

    .sidebar-banner.loading .skeleton {
        height: 200px;
        background: linear-gradient(90deg, #f0f0f0 25%, #e0e0e0 50%, #f0f0f0 75%);
        background-size: 200% 100%;
        animation: shimmer 1.5s infinite;
        border-radius: 8px;
    }

    @keyframes shimmer {
        0% { background-position: -200% 0; }
        100% { background-position: 200% 0; }
    }

    .banner-item {
        display: block;
        border-radius: 8px;
        overflow: hidden;
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
        transition: box-shadow 0.2s, transform 0.2s;
    }

    .banner-item:hover {
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
        transform: translateY(-2px);
    }

    .banner-item img {
        width: 100%;
        height: auto;
        display: block;
    }

    .text-banner {
        padding: 20px;
        background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        color: #fff;
        font-weight: 500;
        text-align: center;
    }
</style>
