<script>
    import { onMount } from 'svelte';
    import { api } from '@angple/admin';

    let banners = [];
    let loading = true;
    let showAddModal = false;
    let editingBanner = null;

    // 필터
    let filterPosition = '';
    let filterActive = '';

    // 새 배너 폼
    let newBanner = {
        title: '',
        image_url: '',
        link_url: '',
        position: 'sidebar',
        start_date: '',
        end_date: '',
        priority: 0,
        is_active: true,
        alt_text: '',
        target: '_blank',
        memo: ''
    };

    onMount(async () => {
        await loadBanners();
    });

    async function loadBanners() {
        loading = true;
        try {
            let url = '/plugins/banner/admin/list';
            const params = [];
            if (filterPosition) params.push(`position=${filterPosition}`);
            if (filterActive) params.push(`is_active=${filterActive}`);
            if (params.length > 0) url += '?' + params.join('&');

            const res = await api.get(url);
            banners = res.banners || [];
        } catch (e) {
            console.error('Failed to load banners:', e);
        } finally {
            loading = false;
        }
    }

    async function createBanner() {
        try {
            const payload = {
                ...newBanner,
                start_date: newBanner.start_date ? new Date(newBanner.start_date).toISOString() : null,
                end_date: newBanner.end_date ? new Date(newBanner.end_date).toISOString() : null
            };
            await api.post('/plugins/banner/admin', payload);
            showAddModal = false;
            resetForm();
            await loadBanners();
        } catch (e) {
            alert('배너 추가 실패: ' + e.message);
        }
    }

    async function updateBanner() {
        if (!editingBanner) return;
        try {
            const payload = {
                ...editingBanner,
                start_date: editingBanner.start_date ? new Date(editingBanner.start_date).toISOString() : null,
                end_date: editingBanner.end_date ? new Date(editingBanner.end_date).toISOString() : null
            };
            await api.put(`/plugins/banner/admin/${editingBanner.id}`, payload);
            editingBanner = null;
            await loadBanners();
        } catch (e) {
            alert('배너 수정 실패: ' + e.message);
        }
    }

    async function toggleActive(banner) {
        try {
            await api.put(`/plugins/banner/admin/${banner.id}`, {
                is_active: !banner.is_active
            });
            await loadBanners();
        } catch (e) {
            alert('상태 변경 실패: ' + e.message);
        }
    }

    async function deleteBanner(id) {
        if (!confirm('정말 삭제하시겠습니까?')) return;
        try {
            await api.delete(`/plugins/banner/admin/${id}`);
            await loadBanners();
        } catch (e) {
            alert('삭제 실패: ' + e.message);
        }
    }

    function resetForm() {
        newBanner = {
            title: '',
            image_url: '',
            link_url: '',
            position: 'sidebar',
            start_date: '',
            end_date: '',
            priority: 0,
            is_active: true,
            alt_text: '',
            target: '_blank',
            memo: ''
        };
    }

    function formatDate(dateStr) {
        if (!dateStr) return '-';
        return new Date(dateStr).toLocaleDateString('ko-KR');
    }

    function getPositionLabel(position) {
        const labels = {
            header: '헤더',
            sidebar: '사이드바',
            content: '콘텐츠',
            footer: '푸터'
        };
        return labels[position] || position;
    }

    // 통계 계산
    $: stats = {
        total: banners.length,
        active: banners.filter(b => b.is_active).length,
        totalClicks: banners.reduce((sum, b) => sum + (b.click_count || 0), 0),
        totalViews: banners.reduce((sum, b) => sum + (b.view_count || 0), 0)
    };
</script>

<div class="banner-admin">
    <header class="page-header">
        <h1>배너 광고 관리</h1>
        <button class="btn-primary" on:click={() => showAddModal = true}>
            + 배너 추가
        </button>
    </header>

    <div class="stats-cards">
        <div class="stat-card">
            <span class="stat-value">{stats.total}</span>
            <span class="stat-label">전체 배너</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">{stats.active}</span>
            <span class="stat-label">활성 배너</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">{stats.totalViews.toLocaleString()}</span>
            <span class="stat-label">총 노출수</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">{stats.totalClicks.toLocaleString()}</span>
            <span class="stat-label">총 클릭수</span>
        </div>
    </div>

    <div class="filters">
        <select bind:value={filterPosition} on:change={loadBanners}>
            <option value="">전체 위치</option>
            <option value="header">헤더</option>
            <option value="sidebar">사이드바</option>
            <option value="content">콘텐츠</option>
            <option value="footer">푸터</option>
        </select>
        <select bind:value={filterActive} on:change={loadBanners}>
            <option value="">전체 상태</option>
            <option value="true">활성</option>
            <option value="false">비활성</option>
        </select>
    </div>

    {#if loading}
        <div class="loading">로딩 중...</div>
    {:else}
        <table class="data-table">
            <thead>
                <tr>
                    <th>ID</th>
                    <th>미리보기</th>
                    <th>제목</th>
                    <th>위치</th>
                    <th>기간</th>
                    <th>우선순위</th>
                    <th>노출/클릭</th>
                    <th>상태</th>
                    <th>관리</th>
                </tr>
            </thead>
            <tbody>
                {#each banners as banner}
                    <tr class:inactive={!banner.is_active}>
                        <td>{banner.id}</td>
                        <td class="preview-cell">
                            {#if banner.image_url}
                                <img src={banner.image_url} alt={banner.title} class="preview-img" />
                            {:else}
                                <span class="no-image">-</span>
                            {/if}
                        </td>
                        <td>
                            <div class="title-cell">
                                <span class="title">{banner.title}</span>
                                {#if banner.link_url}
                                    <a href={banner.link_url} target="_blank" class="link-preview">
                                        {banner.link_url.substring(0, 30)}...
                                    </a>
                                {/if}
                            </div>
                        </td>
                        <td>
                            <span class="badge position-{banner.position}">
                                {getPositionLabel(banner.position)}
                            </span>
                        </td>
                        <td>
                            {formatDate(banner.start_date)} ~ {formatDate(banner.end_date)}
                        </td>
                        <td>{banner.priority}</td>
                        <td>
                            <span class="stats-cell">
                                {banner.view_count?.toLocaleString() || 0} / {banner.click_count?.toLocaleString() || 0}
                            </span>
                        </td>
                        <td>
                            <button
                                class="toggle-btn"
                                class:active={banner.is_active}
                                on:click={() => toggleActive(banner)}
                            >
                                {banner.is_active ? '활성' : '비활성'}
                            </button>
                        </td>
                        <td class="actions">
                            <button class="btn-sm" on:click={() => editingBanner = {...banner}}>수정</button>
                            <button class="btn-sm danger" on:click={() => deleteBanner(banner.id)}>삭제</button>
                        </td>
                    </tr>
                {/each}
            </tbody>
        </table>
    {/if}
</div>

<!-- 배너 추가 모달 -->
{#if showAddModal}
    <div class="modal-overlay" on:click={() => showAddModal = false}>
        <div class="modal" on:click|stopPropagation>
            <h2>배너 추가</h2>
            <form on:submit|preventDefault={createBanner}>
                <div class="form-row">
                    <div class="form-group">
                        <label>제목 *</label>
                        <input type="text" bind:value={newBanner.title} required />
                    </div>
                    <div class="form-group">
                        <label>위치 *</label>
                        <select bind:value={newBanner.position} required>
                            <option value="header">헤더</option>
                            <option value="sidebar">사이드바</option>
                            <option value="content">콘텐츠</option>
                            <option value="footer">푸터</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label>이미지 URL</label>
                    <input type="url" bind:value={newBanner.image_url} placeholder="https://..." />
                </div>
                <div class="form-group">
                    <label>링크 URL</label>
                    <input type="url" bind:value={newBanner.link_url} placeholder="https://..." />
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>시작일</label>
                        <input type="date" bind:value={newBanner.start_date} />
                    </div>
                    <div class="form-group">
                        <label>종료일</label>
                        <input type="date" bind:value={newBanner.end_date} />
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>우선순위</label>
                        <input type="number" bind:value={newBanner.priority} min="0" />
                    </div>
                    <div class="form-group">
                        <label>링크 타겟</label>
                        <select bind:value={newBanner.target}>
                            <option value="_blank">새 탭</option>
                            <option value="_self">현재 탭</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label>대체 텍스트</label>
                    <input type="text" bind:value={newBanner.alt_text} />
                </div>
                <div class="form-group">
                    <label>
                        <input type="checkbox" bind:checked={newBanner.is_active} />
                        활성화
                    </label>
                </div>
                <div class="form-group">
                    <label>메모</label>
                    <textarea bind:value={newBanner.memo}></textarea>
                </div>
                <div class="form-actions">
                    <button type="button" on:click={() => showAddModal = false}>취소</button>
                    <button type="submit" class="btn-primary">추가</button>
                </div>
            </form>
        </div>
    </div>
{/if}

<!-- 배너 수정 모달 -->
{#if editingBanner}
    <div class="modal-overlay" on:click={() => editingBanner = null}>
        <div class="modal" on:click|stopPropagation>
            <h2>배너 수정</h2>
            <form on:submit|preventDefault={updateBanner}>
                <div class="form-row">
                    <div class="form-group">
                        <label>제목 *</label>
                        <input type="text" bind:value={editingBanner.title} required />
                    </div>
                    <div class="form-group">
                        <label>위치 *</label>
                        <select bind:value={editingBanner.position} required>
                            <option value="header">헤더</option>
                            <option value="sidebar">사이드바</option>
                            <option value="content">콘텐츠</option>
                            <option value="footer">푸터</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label>이미지 URL</label>
                    <input type="url" bind:value={editingBanner.image_url} />
                </div>
                <div class="form-group">
                    <label>링크 URL</label>
                    <input type="url" bind:value={editingBanner.link_url} />
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>시작일</label>
                        <input type="date" bind:value={editingBanner.start_date} />
                    </div>
                    <div class="form-group">
                        <label>종료일</label>
                        <input type="date" bind:value={editingBanner.end_date} />
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label>우선순위</label>
                        <input type="number" bind:value={editingBanner.priority} min="0" />
                    </div>
                    <div class="form-group">
                        <label>링크 타겟</label>
                        <select bind:value={editingBanner.target}>
                            <option value="_blank">새 탭</option>
                            <option value="_self">현재 탭</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label>대체 텍스트</label>
                    <input type="text" bind:value={editingBanner.alt_text} />
                </div>
                <div class="form-group">
                    <label>메모</label>
                    <textarea bind:value={editingBanner.memo}></textarea>
                </div>
                <div class="form-actions">
                    <button type="button" on:click={() => editingBanner = null}>취소</button>
                    <button type="submit" class="btn-primary">수정</button>
                </div>
            </form>
        </div>
    </div>
{/if}

<style>
    .banner-admin {
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
        grid-template-columns: repeat(4, 1fr);
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
        font-size: 28px;
        font-weight: 700;
        color: #2563eb;
    }

    .stat-label {
        font-size: 13px;
        color: #6b7280;
    }

    .filters {
        display: flex;
        gap: 12px;
        margin-bottom: 16px;
    }

    .filters select {
        padding: 8px 12px;
        border: 1px solid #d1d5db;
        border-radius: 6px;
        font-size: 14px;
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

    .preview-cell {
        width: 80px;
    }

    .preview-img {
        width: 60px;
        height: 40px;
        object-fit: cover;
        border-radius: 4px;
    }

    .no-image {
        color: #9ca3af;
    }

    .title-cell .title {
        display: block;
        font-weight: 500;
    }

    .title-cell .link-preview {
        font-size: 11px;
        color: #6b7280;
    }

    .badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: 4px;
        font-size: 12px;
        background: #e5e7eb;
    }

    .badge.position-header { background: #dbeafe; color: #1e40af; }
    .badge.position-sidebar { background: #d1fae5; color: #065f46; }
    .badge.position-content { background: #fef3c7; color: #92400e; }
    .badge.position-footer { background: #f3e8ff; color: #6b21a8; }

    .stats-cell {
        font-size: 12px;
        color: #6b7280;
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

    .actions {
        display: flex;
        gap: 8px;
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

    .btn-sm.danger {
        color: #dc2626;
        border-color: #fecaca;
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
        max-width: 560px;
        max-height: 90vh;
        overflow-y: auto;
    }

    .modal h2 {
        margin: 0 0 20px 0;
    }

    .form-row {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 16px;
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
    .form-group input[type="url"],
    .form-group input[type="number"],
    .form-group input[type="date"],
    .form-group select,
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
