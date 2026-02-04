<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/core';

    export let user;

    let posts = [];
    let loading = true;
    let error = null;

    onMount(async () => {
        try {
            const res = await api.get('/plugins/promotion/posts?limit=5');
            posts = res.posts || [];
        } catch (e) {
            error = e.message;
        } finally {
            loading = false;
        }
    });
</script>

<div class="promotion-widget">
    <h3 class="widget-title">직접홍보 최신글</h3>

    {#if loading}
        <div class="loading">로딩 중...</div>
    {:else if error}
        <div class="error">{error}</div>
    {:else if posts.length === 0}
        <div class="empty">등록된 글이 없습니다</div>
    {:else}
        <ul class="post-list">
            {#each posts as post}
                <li class="post-item" class:pinned={post.is_pinned}>
                    <a href="/promotion/{post.id}">
                        {#if post.is_pinned}
                            <span class="pin-icon">📌</span>
                        {/if}
                        <span class="title">{post.title}</span>
                    </a>
                    <span class="meta">
                        <span class="author">{post.author_name}</span>
                        <span class="views">{post.views}</span>
                    </span>
                </li>
            {/each}
        </ul>
        <a href="/promotion" class="view-all">전체보기</a>
    {/if}
</div>

<style>
    .promotion-widget {
        background: #fff;
        border-radius: 8px;
        padding: 16px;
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
    }

    .widget-title {
        font-size: 14px;
        font-weight: 600;
        margin: 0 0 12px 0;
        padding-bottom: 8px;
        border-bottom: 1px solid #e5e7eb;
    }

    .loading, .error, .empty {
        padding: 20px;
        text-align: center;
        color: #6b7280;
        font-size: 13px;
    }

    .error {
        color: #ef4444;
    }

    .post-list {
        list-style: none;
        margin: 0;
        padding: 0;
    }

    .post-item {
        padding: 8px 0;
        border-bottom: 1px solid #f3f4f6;
    }

    .post-item:last-child {
        border-bottom: none;
    }

    .post-item.pinned {
        background: #fffbeb;
        margin: 0 -8px;
        padding: 8px;
        border-radius: 4px;
    }

    .post-item a {
        display: block;
        color: #374151;
        text-decoration: none;
        font-size: 13px;
        line-height: 1.4;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .post-item a:hover {
        color: #2563eb;
    }

    .pin-icon {
        margin-right: 4px;
    }

    .meta {
        display: flex;
        justify-content: space-between;
        font-size: 11px;
        color: #9ca3af;
        margin-top: 4px;
    }

    .view-all {
        display: block;
        text-align: center;
        padding: 10px;
        margin-top: 8px;
        font-size: 12px;
        color: #6b7280;
        text-decoration: none;
        border-top: 1px solid #e5e7eb;
    }

    .view-all:hover {
        color: #2563eb;
    }
</style>
