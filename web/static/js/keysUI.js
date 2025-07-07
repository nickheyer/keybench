// Keys UI management
let currentKeyPage = 1;
const keysPerPage = 5;
let allKeys = [];
let keyFilter = '';

async function refreshKeys() {
    try {
        const response = await fetch(`${API_BASE}/keys`);
        allKeys = await response.json() || [];
        displayKeys();
    } catch (error) {
        console.error('Failed to load keys:', error);
    }
}

function displayKeys() {
    const keysList = document.getElementById('keysList');
    
    // Filter keys
    let filteredKeys = allKeys;
    if (keyFilter) {
        filteredKeys = allKeys.filter(key => 
            key.type.toLowerCase().includes(keyFilter.toLowerCase()) ||
            key.id.includes(keyFilter) ||
            key.benchmark_id.includes(keyFilter)
        );
    }
    
    if (filteredKeys.length === 0) {
        keysList.innerHTML = `
            <div class="empty-state">
                <p>No keys found</p>
                ${keyFilter ? '<button class="btn btn-small btn-secondary" onclick="keyFilter=\'\'; displayKeys()">Clear Filter</button>' : ''}
            </div>
        `;
        return;
    }
    
    // Group keys by benchmark
    const keysByBenchmark = {};
    filteredKeys.forEach(key => {
        if (!keysByBenchmark[key.benchmark_id]) {
            keysByBenchmark[key.benchmark_id] = [];
        }
        keysByBenchmark[key.benchmark_id].push(key);
    });
    
    // Pagination
    const benchmarkIds = Object.keys(keysByBenchmark);
    const totalPages = Math.ceil(benchmarkIds.length / keysPerPage);
    const startIdx = (currentKeyPage - 1) * keysPerPage;
    const endIdx = startIdx + keysPerPage;
    const pageBenchmarks = benchmarkIds.slice(startIdx, endIdx);
    
    let html = `
        <div class="keys-header">
            <div class="keys-stats">
                <span><strong>${filteredKeys.length}</strong> keys in <strong>${benchmarkIds.length}</strong> benchmarks</span>
            </div>
            <div class="keys-filter">
                <input type="text" 
                       placeholder="Filter by type, ID..." 
                       value="${keyFilter}" 
                       onchange="keyFilter = this.value; currentKeyPage = 1; displayKeys()"
                       class="filter-input">
            </div>
        </div>
    `;
    
    // Display grouped keys
    pageBenchmarks.forEach(benchmarkId => {
        const benchKeys = keysByBenchmark[benchmarkId];
        const firstKey = benchKeys[0];
        
        // Count by type
        const typeCounts = {};
        benchKeys.forEach(k => {
            typeCounts[k.type] = (typeCounts[k.type] || 0) + 1;
        });
        
        html += `
            <div class="benchmark-group">
                <div class="benchmark-group-header" onclick="toggleBenchmarkGroup('${benchmarkId}')">
                    <span class="group-title">
                        <i class="arrow">▶</i>
                        Benchmark ${benchmarkId.substring(0, 8)}
                        <span class="badge">${benchKeys.length} keys</span>
                        <span class="type-badges">
                            ${Object.entries(typeCounts).map(([type, count]) => 
                                `<span class="type-badge ${type.toLowerCase()}">${type} ×${count}</span>`
                            ).join('')}
                        </span>
                    </span>
                    <span class="group-meta">
                        ${new Date(firstKey.created_at).toLocaleString()}
                    </span>
                </div>
                <div class="benchmark-group-content" id="group-${benchmarkId}" style="display: none;">
                    <div class="group-actions">
                        <button class="btn btn-small btn-secondary" onclick="downloadBenchmarkKeys('${benchmarkId}')">
                            📥 Download All
                        </button>
                        <button class="btn btn-small btn-danger" onclick="deleteBenchmarkKeys('${benchmarkId}')">
                            🗑️ Delete All
                        </button>
                    </div>
                    <div class="keys-table">
                        <table>
                            <thead>
                                <tr>
                                    <th>Type</th>
                                    <th>Size</th>
                                    <th>Key ID</th>
                                    <th>Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${benchKeys.map(key => `
                                    <tr>
                                        <td><span class="key-type-badge ${key.type.toLowerCase()}">${key.type}</span></td>
                                        <td>${key.size} bits</td>
                                        <td class="mono">${key.id.substring(0, 12)}...</td>
                                        <td class="actions">
                                            <button class="btn-icon" onclick="downloadKey('${key.id}', 'public')" title="Public Key">
                                                <span>🔓</span>
                                            </button>
                                            <button class="btn-icon" onclick="downloadKey('${key.id}', 'private')" title="Private Key">
                                                <span>🔐</span>
                                            </button>
                                            <button class="btn-icon" onclick="viewKey('${key.id}')" title="View">
                                                <span>👁️</span>
                                            </button>
                                            <button class="btn-icon danger" onclick="deleteKey('${key.id}')" title="Delete">
                                                <span>🗑️</span>
                                            </button>
                                        </td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;
    });
    
    // Pagination controls
    if (totalPages > 1) {
        html += '<div class="pagination">';
        if (currentKeyPage > 1) {
            html += `<button class="page-btn" onclick="currentKeyPage--; displayKeys()">‹</button>`;
        }
        
        let startPage = Math.max(1, currentKeyPage - 2);
        let endPage = Math.min(totalPages, currentKeyPage + 2);
        
        for (let i = startPage; i <= endPage; i++) {
            html += `<button class="page-btn ${i === currentKeyPage ? 'active' : ''}" 
                             onclick="currentKeyPage = ${i}; displayKeys()">${i}</button>`;
        }
        
        if (currentKeyPage < totalPages) {
            html += `<button class="page-btn" onclick="currentKeyPage++; displayKeys()">›</button>`;
        }
        html += '</div>';
    }
    
    keysList.innerHTML = html;
}

function toggleBenchmarkGroup(benchmarkId) {
    const content = document.getElementById(`group-${benchmarkId}`);
    const arrow = content.parentElement.querySelector('.arrow');
    
    if (content.style.display === 'none') {
        content.style.display = 'block';
        arrow.style.transform = 'rotate(90deg)';
    } else {
        content.style.display = 'none';
        arrow.style.transform = 'rotate(0deg)';
    }
}

async function viewKey(keyId) {
    try {
        const response = await fetch(`${API_BASE}/keys/${keyId}`);
        const key = await response.json();
        
        // Create modal
        const modal = document.createElement('div');
        modal.className = 'modal';
        modal.innerHTML = `
            <div class="modal-content">
                <div class="modal-header">
                    <h3>Key Details</h3>
                    <button class="close-btn" onclick="this.closest('.modal').remove()">×</button>
                </div>
                <div class="modal-body">
                    <div class="key-detail">
                        <label>Type:</label> ${key.type}
                    </div>
                    <div class="key-detail">
                        <label>Size:</label> ${key.size} bits
                    </div>
                    <div class="key-detail">
                        <label>ID:</label> <span class="mono">${key.id}</span>
                    </div>
                    <div class="key-detail">
                        <label>Created:</label> ${new Date(key.created_at).toLocaleString()}
                    </div>
                    <div class="key-detail">
                        <label>Benchmark ID:</label> <span class="mono">${key.benchmark_id}</span>
                    </div>
                    
                    <div class="key-content">
                        <h4>Public Key</h4>
                        <pre>${key.public_key}</pre>
                        <button class="btn btn-small btn-secondary" onclick="copyToClipboard('${btoa(key.public_key)}')">
                            Copy Public Key
                        </button>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
    } catch (error) {
        console.error('Failed to view key:', error);
    }
}

function copyToClipboard(base64Text) {
    const text = atob(base64Text);
    navigator.clipboard.writeText(text).then(() => {
        alert('Copied to clipboard!');
    });
}

async function downloadBenchmarkKeys(benchmarkId) {
    const keys = allKeys.filter(k => k.benchmark_id === benchmarkId);
    
    if (confirm(`Download ${keys.length * 2} key files?`)) {
        for (const key of keys) {
            await downloadKey(key.id, 'public');
            await new Promise(resolve => setTimeout(resolve, 200));
            await downloadKey(key.id, 'private');
            await new Promise(resolve => setTimeout(resolve, 200));
        }
    }
}

async function deleteBenchmarkKeys(benchmarkId) {
    const keys = allKeys.filter(k => k.benchmark_id === benchmarkId);
    
    if (!confirm(`Delete all ${keys.length} keys from this benchmark? This cannot be undone.`)) {
        return;
    }
    
    for (const key of keys) {
        await fetch(`${API_BASE}/keys/${key.id}`, { method: 'DELETE' });
    }
    
    refreshKeys();
}