const API_BASE = '/api/v1';

// Local storage keys
const STORAGE_KEYS = {
    BENCHMARK_HISTORY: 'keybench_history',
    GENERATED_KEYS: 'keybench_keys'
};

// Initialize the application
document.addEventListener('DOMContentLoaded', () => {
    loadSystemInfo();
    loadBenchmarkHistory();
    refreshKeys();
    
    document.getElementById('benchmarkForm').addEventListener('submit', startBenchmark);
});

async function loadSystemInfo() {
    try {
        const response = await fetch(`${API_BASE}/system-info`);
        const data = await response.json();
        
        const systemInfoDiv = document.getElementById('systemInfo');
        systemInfoDiv.innerHTML = `
            <h2>System Information</h2>
            <div class="system-info-grid">
                <div class="info-item">
                    <label>OS</label>
                    <div class="value">${data.os} ${data.architecture}</div>
                </div>
                <div class="info-item">
                    <label>CPU</label>
                    <div class="value">${data.cpu_model}</div>
                </div>
                <div class="info-item">
                    <label>Available Workers</label>
                    <div class="value">${data.cpu_threads} threads</div>
                </div>
                <div class="info-item">
                    <label>Memory</label>
                    <div class="value">${(data.total_memory / (1024**3)).toFixed(2)} GB</div>
                </div>
                <div class="info-item">
                    <label>Go Version</label>
                    <div class="value">${data.go_version}</div>
                </div>
                <div class="info-item">
                    <label>Load Average</label>
                    <div class="value">${data.load_average.toFixed(2)}</div>
                </div>
            </div>
        `;
    } catch (error) {
        console.error('Failed to load system info:', error);
    }
}

async function startBenchmark(event) {
    event.preventDefault();
    
    const formData = new FormData(event.target);
    const algorithms = Array.from(formData.getAll('algorithms'));
    const keySizesInput = formData.get('keySizes');
    const keySizes = keySizesInput.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
    
    console.log('Form data:', {
        algorithms: algorithms,
        keySizesInput: keySizesInput,
        keySizes: keySizes
    });
    
    if (keySizes.length === 0) {
        alert('Please enter valid key sizes');
        return;
    }
    
    const config = {
        algorithms: algorithms,
        key_sizes: keySizes,
        iterations: parseInt(formData.get('iterations')),
        parallel: parseInt(formData.get('parallel')),
        workers: parseInt(formData.get('workers')) || 1,
        timeout: parseInt(formData.get('timeout')),
        file_storage: document.getElementById('fileStorage').checked,
        show_progress: true,
        verbose: true
    };
    
    try {
        const response = await fetch(`${API_BASE}/benchmarks`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });
        
        const data = await response.json();
        
        // Show progress section
        document.getElementById('benchmarkProgress').style.display = 'block';
        document.getElementById('resultsSection').style.display = 'none';
        
        // Start monitoring progress
        monitorBenchmark(data.job_id);
        
    } catch (error) {
        console.error('Failed to start benchmark:', error);
        alert('Failed to start benchmark: ' + error.message);
    }
}

let currentJobId = null;
let currentWebSocket = null;

async function monitorBenchmark(jobId) {
    currentJobId = jobId;
    const progressBar = document.getElementById('progressBar');
    const progressInfo = document.getElementById('progressInfo');
    
    // Connect via WebSocket for real-time progress updates
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${window.location.host}${API_BASE}/benchmarks/${jobId}/progress`);
    currentWebSocket = ws;
    
    ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        
        if (data.completed) {
            ws.close();
            currentWebSocket = null;
            currentJobId = null;
            progressBar.style.width = '100%';
            progressBar.textContent = '100%';
            
            if (data.status === 'completed') {
                progressInfo.textContent = 'Benchmark completed!';
                // Fetch full results
                fetch(`${API_BASE}/benchmarks/${jobId}`)
                    .then(res => res.json())
                    .then(job => {
                        showResults(job);
                        refreshKeys();
                        loadBenchmarkHistory();
                    });
            } else if (data.status === 'failed') {
                progressInfo.textContent = 'Benchmark failed!';
                progressInfo.style.color = 'var(--danger-color)';
            } else if (data.status === 'terminated') {
                progressInfo.textContent = 'Benchmark terminated by user';
                progressInfo.style.color = 'var(--warning-color)';
                document.getElementById('benchmarkProgress').style.display = 'none';
            }
        } else if (data.percentage !== undefined) {
            // Update progress bar and info
            const percentage = Math.round(data.percentage);
            progressBar.style.width = `${percentage}%`;
            progressBar.textContent = `${percentage}%`;
            
            if (data.algorithm && data.keySize) {
                progressInfo.innerHTML = `
                    <div>Processing: ${data.algorithm} - ${data.keySize} bits</div>
                    <div>Progress: ${data.current}/${data.total} iterations</div>
                    <div>Rate: ${data.rate.toFixed(2)} keys/sec</div>
                `;
            }
        }
    };
    
    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        // Fallback to polling
        ws.close();
        pollBenchmarkStatus(jobId);
    };
    
    ws.onclose = () => {
        // If WebSocket closes unexpectedly, fallback to polling
        if (progressBar.style.width !== '100%') {
            pollBenchmarkStatus(jobId);
        }
    };
}

// Fallback polling method
async function pollBenchmarkStatus(jobId) {
    const progressBar = document.getElementById('progressBar');
    const progressInfo = document.getElementById('progressInfo');
    
    const pollInterval = setInterval(async () => {
        try {
            const response = await fetch(`${API_BASE}/benchmarks/${jobId}`);
            const job = await response.json();
            
            if (job.status === 'completed') {
                clearInterval(pollInterval);
                progressBar.style.width = '100%';
                progressBar.textContent = '100%';
                progressInfo.textContent = 'Benchmark completed!';
                
                console.log('Job completed:', job);
                showResults(job);
                refreshKeys();
                loadBenchmarkHistory();
                
            } else if (job.status === 'failed') {
                clearInterval(pollInterval);
                progressInfo.textContent = `Benchmark failed: ${job.error}`;
                progressInfo.style.color = 'var(--danger-color)';
            }
        } catch (error) {
            console.error('Failed to poll benchmark status:', error);
        }
    }, 1000);
}

function showResults(job) {
    const resultsSection = document.getElementById('resultsSection');
    const resultsDiv = document.getElementById('results');
    
    resultsSection.style.display = 'block';
    
    let html = `
        <div class="benchmark-summary">
            <p><strong>Job ID:</strong> ${job.id}</p>
            <p><strong>Started:</strong> ${new Date(job.started_at).toLocaleString()}</p>
            <p><strong>Completed:</strong> ${new Date(job.completed_at).toLocaleString()}</p>
        </div>
        <table class="results-table">
            <thead>
                <tr>
                    <th>Algorithm</th>
                    <th>Key Size</th>
                    <th>Iterations</th>
                    <th>Total Time</th>
                    <th>Avg Time</th>
                    <th>Keys/Sec</th>
                    <th>Errors</th>
                    <th>Keys Generated</th>
                </tr>
            </thead>
            <tbody>
    `;
    
    if (job.results && job.results.length > 0) {
        job.results.forEach(result => {
            console.log('Result:', result); // Debug logging
            html += `
                <tr>
                    <td>${result.algorithm}</td>
                    <td>${result.key_size}</td>
                    <td>${result.iterations * result.parallel}</td>
                    <td>${formatDuration(result.total_time)}</td>
                    <td>${formatDuration(result.average_time)}</td>
                    <td>${result.keys_per_second ? result.keys_per_second.toFixed(2) : '0'}</td>
                    <td>${result.errors || 0}</td>
                    <td>${result.key_ids ? result.key_ids.length : 0}</td>
                </tr>
            `;
        });
    } else {
        html += '<tr><td colspan="8" style="text-align: center;">No results available</td></tr>';
    }
    
    html += '</tbody></table>';
    resultsDiv.innerHTML = html;
}

async function refreshKeys() {
    try {
        const response = await fetch(`${API_BASE}/keys`);
        const keys = await response.json();
        
        const keysList = document.getElementById('keysList');
        
        if (!keys || keys.length === 0) {
            keysList.innerHTML = '<p style="text-align: center; color: var(--text-secondary);">No keys generated yet</p>';
            return;
        }
        
        let html = '';
        keys.forEach(key => {
            html += `
                <div class="key-item">
                    <div class="key-info">
                        <h4>${key.type} - ${key.size} bits</h4>
                        <div class="meta">
                            ID: ${key.id.substring(0, 8)}... | 
                            Created: ${new Date(key.created_at).toLocaleString()}
                        </div>
                    </div>
                    <div class="key-actions">
                        <button class="btn btn-small btn-secondary" onclick="downloadKey('${key.id}', 'public')">
                            Public Key
                        </button>
                        <button class="btn btn-small btn-secondary" onclick="downloadKey('${key.id}', 'private')">
                            Private Key
                        </button>
                        <button class="btn btn-small btn-danger" onclick="deleteKey('${key.id}')">
                            Delete
                        </button>
                    </div>
                </div>
            `;
        });
        
        keysList.innerHTML = html;
    } catch (error) {
        console.error('Failed to load keys:', error);
    }
}

async function downloadKey(keyId, keyType) {
    window.location.href = `${API_BASE}/keys/${keyId}/download?type=${keyType}`;
}

async function deleteKey(keyId) {
    if (!confirm('Are you sure you want to delete this key?')) {
        return;
    }
    
    try {
        await fetch(`${API_BASE}/keys/${keyId}`, {
            method: 'DELETE'
        });
        refreshKeys();
    } catch (error) {
        console.error('Failed to delete key:', error);
        alert('Failed to delete key');
    }
}

async function clearAllKeys() {
    if (!confirm('Are you sure you want to delete ALL keys and their files? This action cannot be undone.')) {
        return;
    }
    
    try {
        // Use the new cleanup endpoint that deletes both keys and files
        const response = await fetch(`${API_BASE}/keys/cleanup-all`, {
            method: 'POST'
        });
        
        if (response.ok) {
            const result = await response.json();
            alert(result.message);
        } else {
            throw new Error('Failed to cleanup keys');
        }
        
        refreshKeys();
    } catch (error) {
        console.error('Failed to clear keys:', error);
        alert('Failed to clear all keys and files');
    }
}

async function loadBenchmarkHistory() {
    try {
        const response = await fetch(`${API_BASE}/benchmarks`);
        const benchmarks = await response.json();
        
        const historyDiv = document.getElementById('benchmarkHistory');
        
        if (!benchmarks || benchmarks.length === 0) {
            historyDiv.innerHTML = '<p style="text-align: center; color: var(--text-secondary);">No benchmark history</p>';
            return;
        }
        
        let html = '<div class="benchmark-history">';
        benchmarks.reverse().forEach(job => {
            if (!job.config) return;
            
            html += `
                <div class="history-item" onclick="showBenchmarkDetails('${job.id}')">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div>
                            <strong>${job.config.algorithms ? job.config.algorithms.join(', ') : 'Unknown'}</strong>
                            <div class="meta">
                                ${new Date(job.started_at).toLocaleString()} | 
                                ${job.config.iterations || 0} iterations × ${job.config.parallel || 1} workers
                            </div>
                        </div>
                        <span class="status ${job.status}">${job.status}</span>
                    </div>
                </div>
            `;
        });
        html += '</div>';
        
        historyDiv.innerHTML = html;
    } catch (error) {
        console.error('Failed to load benchmark history:', error);
    }
}

async function showBenchmarkDetails(jobId) {
    try {
        const response = await fetch(`${API_BASE}/benchmarks/${jobId}`);
        const job = await response.json();
        
        if (job.status === 'completed') {
            showResults(job);
            document.getElementById('resultsSection').scrollIntoView({ behavior: 'smooth' });
        }
    } catch (error) {
        console.error('Failed to load benchmark details:', error);
    }
}

function formatDuration(nanoseconds) {
    if (nanoseconds < 1000) {
        return `${nanoseconds}ns`;
    } else if (nanoseconds < 1000000) {
        return `${(nanoseconds / 1000).toFixed(2)}µs`;
    } else if (nanoseconds < 1000000000) {
        return `${(nanoseconds / 1000000).toFixed(2)}ms`;
    } else {
        return `${(nanoseconds / 1000000000).toFixed(2)}s`;
    }
}

async function terminateBenchmark() {
    if (!currentJobId) {
        return;
    }
    
    if (confirm('Are you sure you want to terminate this benchmark? Any keys being generated will be deleted.')) {
        try {
            const response = await fetch(`${API_BASE}/benchmarks/${currentJobId}/terminate`, {
                method: 'POST'
            });
            
            if (response.ok) {
                if (currentWebSocket) {
                    currentWebSocket.close();
                    currentWebSocket = null;
                }
                currentJobId = null;
            }
        } catch (error) {
            console.error('Failed to terminate benchmark:', error);
            alert('Failed to terminate benchmark: ' + error.message);
        }
    }
}
