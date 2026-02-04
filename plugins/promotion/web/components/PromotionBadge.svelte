<script>
    export let post;
</script>

{#if post.isPromotion}
    <span class="promotion-badge" class:pinned={post.isPinned}>
        AD
    </span>
{/if}

<style>
    .promotion-badge {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        padding: 2px 6px;
        margin-right: 6px;
        font-size: 10px;
        font-weight: 600;
        color: #fff;
        background-color: #6b7280;
        border-radius: 4px;
        text-transform: uppercase;
        letter-spacing: 0.5px;
    }

    .promotion-badge.pinned {
        background-color: #f59e0b;
    }
</style>
