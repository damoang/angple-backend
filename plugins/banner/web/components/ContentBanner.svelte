<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/core';

    export let post;

    let banner = null;
    let loading = true;

    onMount(async () => {
        try {
            const res = await api.get('/plugins/banner/list?position=content');
            if (res.banners && res.banners.length > 0) {
                banner = res.banners[0];
                // 노출 트래킹
                api.post(`/plugins/banner/${banner.id}/view`).catch(() => {});
            }
        } catch (e) {
            console.error('Failed to load content banner:', e);
        } finally {
            loading = false;
        }
    });
</script>

{#if !loading && banner}
    <div class="content-banner">
        <span class="ad-label">AD</span>
        <a
            href="/api/plugins/banner/{banner.id}/click"
            target={banner.target || '_blank'}
            rel="noopener noreferrer sponsored"
            class="banner-link"
        >
            {#if banner.image_url}
                <img
                    src={banner.image_url}
                    alt={banner.alt_text || banner.title}
                    loading="lazy"
                />
            {:else}
                <div class="text-content">
                    <span class="title">{banner.title}</span>
                </div>
            {/if}
        </a>
    </div>
{/if}

<style>
    .content-banner {
        position: relative;
        margin: 24px 0;
        padding: 16px;
        background: #f8f9fa;
        border-radius: 8px;
        border: 1px solid #e5e7eb;
    }

    .ad-label {
        position: absolute;
        top: 8px;
        left: 8px;
        padding: 2px 6px;
        font-size: 10px;
        font-weight: 600;
        color: #6b7280;
        background: #fff;
        border-radius: 4px;
        text-transform: uppercase;
    }

    .banner-link {
        display: block;
        text-decoration: none;
        color: inherit;
    }

    .banner-link img {
        width: 100%;
        height: auto;
        display: block;
        border-radius: 4px;
    }

    .text-content {
        padding: 20px;
        text-align: center;
    }

    .text-content .title {
        font-size: 16px;
        font-weight: 500;
        color: #374151;
    }

    .banner-link:hover .title {
        color: #2563eb;
    }
</style>
