<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/admin';

    let advertisers = [];
    let loading = true;
    let showAddModal = false;

    // 새 광고주 폼
    let newAdvertiser = {
        member_id: '',
        name: '',
        post_count: 1,
        is_pinned: false,
        is_active: true,
        memo: ''
    };

    onMount(async () => {
        await loadAdvertisers();
    });

    async function loadAdvertisers() {
        loading = true;
        try {
            const res = await api.get('/plugins/promotion/admin/advertisers');
            advertisers = res || [];
        } catch (e) {
            console.error('Failed to load advertisers:', e);
        } finally {
            loading = false;
        }
    }

    async function createAdvertiser() {
        try {
            await api.post('/plugins/promotion/admin/advertisers', newAdvertiser);
            showAddModal = false;
            newAdvertiser = {
                member_id: '',
                name: '',
                post_count: 1,
                is_pinned: false,
                is_active: true,
                memo: ''
            };
            await loadAdvertisers();
        } catch (e) {
            alert('광고주 추가 실패: ' + e.message);
        }
    }

    async function toggleActive(advertiser) {
        try {
            await api.put(`/plugins/promotion/admin/advertisers/${advertiser.id}`, {
                ...advertiser,
                is_active: !advertiser.is_active
            });
            await loadAdvertisers();
        } catch (e) {
            alert('상태 변경 실패: ' + e.message);
        }
    }

    async function deleteAdvertiser(id) {
        if (!confirm('정말 삭제하시겠습니까? 관련된 모든 글도 삭제됩니다.')) return;

        try {
            await api.delete(`/plugins/promotion/admin/advertisers/${id}`);
            await loadAdvertisers();
        } catch (e) {
            alert('삭제 실패: ' + e.message);
        }
    }

    function formatDate(dateStr) {
        if (!dateStr) return '-';
        return new Date(dateStr).toLocaleDateString('ko-KR');
    }
</script>

<div class="promotion-admin">
    <header class="page-header">
        <h1>직접홍보 관리</h1>
        <button class="btn-primary" on:click={() => showAddModal = true}>
            + 광고주 추가
        </button>
    </header>

    <div class="stats-cards">
        <div class="stat-card">
            <span class="stat-value">{advertisers.length}</span>
            <span class="stat-label">전체 광고주</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">{advertisers.filter(a => a.is_active).length}</span>
            <span class="stat-label">활성 광고주</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">{advertisers.filter(a => a.is_pinned).length}</span>
            <span class="stat-label">상단 고정</span>
        </div>
    </div>

    {#if loading}
        <div class="loading">로딩 중...</div>
    {:else}
        <table class="data-table">
            <thead>
                <tr>
                    <th>ID</th>
                    <th>회원ID</th>
                    <th>광고주명</th>
                    <th>글 개수</th>
                    <th>계약 기간</th>
                    <th>상단고정</th>
                    <th>상태</th>
                    <th>등록일</th>
                    <th>관리</th>
                </tr>
            </thead>
            <tbody>
                {#each advertisers as advertiser}
                    <tr class:inactive={!advertiser.is_active}>
                        <td>{advertiser.id}</td>
                        <td>{advertiser.member_id}</td>
                        <td>{advertiser.name}</td>
                        <td>{advertiser.post_count}</td>
                        <td>
                            {formatDate(advertiser.start_date)} ~ {formatDate(advertiser.end_date)}
                        </td>
                        <td>
                            {#if advertiser.is_pinned}
                                <span class="badge pinned">고정</span>
                            {:else}
                                <span class="badge">-</span>
                            {/if}
                        </td>
                        <td>
                            <button
                                class="toggle-btn"
                                class:active={advertiser.is_active}
                                on:click={() => toggleActive(advertiser)}
                            >
                                {advertiser.is_active ? '활성' : '비활성'}
                            </button>
                        </td>
                        <td>{formatDate(advertiser.created_at)}</td>
                        <td>
                            <button class="btn-sm" on:click={() => deleteAdvertiser(advertiser.id)}>
                                삭제
                            </button>
                        </td>
                    </tr>
                {/each}
            </tbody>
        </table>
    {/if}
</div>

{#if showAddModal}
    <div class="modal-overlay" on:click={() => showAddModal = false}>
        <div class="modal" on:click|stopPropagation>
            <h2>광고주 추가</h2>
            <form on:submit|preventDefault={createAdvertiser}>
                <div class="form-group">
                    <label>회원 ID *</label>
                    <input type="text" bind:value={newAdvertiser.member_id} required />
                </div>
                <div class="form-group">
                    <label>광고주명 *</label>
                    <input type="text" bind:value={newAdvertiser.name} required />
                </div>
                <div class="form-group">
                    <label>표시 글 개수</label>
                    <input type="number" bind:value={newAdvertiser.post_count} min="1" max="10" />
                </div>
                <div class="form-group">
                    <label>
                        <input type="checkbox" bind:checked={newAdvertiser.is_pinned} />
                        상단 고정
                    </label>
                </div>
                <div class="form-group">
                    <label>
                        <input type="checkbox" bind:checked={newAdvertiser.is_active} />
                        활성화
                    </label>
                </div>
                <div class="form-group">
                    <label>메모</label>
                    <textarea bind:value={newAdvertiser.memo}></textarea>
                </div>
                <div class="form-actions">
                    <button type="button" on:click={() => showAddModal = false}>취소</button>
                    <button type="submit" class="btn-primary">추가</button>
                </div>
            </form>
        </div>
    </div>
{/if}

<style>
    .promotion-admin {
        padding: 24px;
    }

    .page-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 24px;
    }

    .page-header h1 {
        font-size: 24px;
        font-weight: 600;
        margin: 0;
    }

    .stats-cards {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 16px;
        margin-bottom: 24px;
    }

    .stat-card {
        background: #fff;
        padding: 20px;
        border-radius: 8px;
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
        text-align: center;
    }

    .stat-value {
        display: block;
        font-size: 32px;
        font-weight: 700;
        color: #2563eb;
    }

    .stat-label {
        font-size: 13px;
        color: #6b7280;
    }

    .data-table {
        width: 100%;
        background: #fff;
        border-radius: 8px;
        box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
        border-collapse: collapse;
    }

    .data-table th,
    .data-table td {
        padding: 12px 16px;
        text-align: left;
        border-bottom: 1px solid #e5e7eb;
    }

    .data-table th {
        background: #f9fafb;
        font-weight: 600;
        font-size: 13px;
    }

    .data-table tr.inactive {
        opacity: 0.5;
    }

    .badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: 4px;
        font-size: 12px;
        background: #e5e7eb;
    }

    .badge.pinned {
        background: #fef3c7;
        color: #92400e;
    }

    .toggle-btn {
        padding: 4px 12px;
        border: none;
        border-radius: 4px;
        cursor: pointer;
        font-size: 12px;
        background: #ef4444;
        color: #fff;
    }

    .toggle-btn.active {
        background: #10b981;
    }

    .btn-primary {
        background: #2563eb;
        color: #fff;
        border: none;
        padding: 8px 16px;
        border-radius: 6px;
        cursor: pointer;
        font-weight: 500;
    }

    .btn-sm {
        padding: 4px 8px;
        font-size: 12px;
        border: 1px solid #d1d5db;
        background: #fff;
        border-radius: 4px;
        cursor: pointer;
    }

    .modal-overlay {
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.5);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 1000;
    }

    .modal {
        background: #fff;
        padding: 24px;
        border-radius: 12px;
        width: 100%;
        max-width: 480px;
    }

    .modal h2 {
        margin: 0 0 20px 0;
    }

    .form-group {
        margin-bottom: 16px;
    }

    .form-group label {
        display: block;
        font-size: 14px;
        font-weight: 500;
        margin-bottom: 6px;
    }

    .form-group input[type="text"],
    .form-group input[type="number"],
    .form-group textarea {
        width: 100%;
        padding: 8px 12px;
        border: 1px solid #d1d5db;
        border-radius: 6px;
        font-size: 14px;
    }

    .form-group textarea {
        min-height: 80px;
        resize: vertical;
    }

    .form-actions {
        display: flex;
        justify-content: flex-end;
        gap: 12px;
        margin-top: 24px;
    }

    .loading {
        text-align: center;
        padding: 40px;
        color: #6b7280;
    }
</style>
