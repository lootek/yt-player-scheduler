async function loadJobs() {
  try {
    const resp = await fetch('/api/jobs');
    if (!resp.ok) throw new Error('fetch failed');
    const jobs = await resp.json();
    renderJobs(jobs);
  } catch (err) {
    console.error('load jobs:', err);
  }
}

function renderJobs(jobs) {
  const tbody = document.querySelector('#jobs-table tbody');
  if (!tbody) return;
  tbody.replaceChildren();

  const highlightId = document.querySelector('#jobs-section')?.dataset.highlightId;

  for (const job of jobs) {
    const tr = document.createElement('tr');
    if (highlightId && job.id === highlightId) {
      tr.style.background = '#fffbcc';
    }

    tr.appendChild(cell(job.id, 'mono'));
    tr.appendChild(cell(job.url));

    const opts = [];
    if (job.mpd) opts.push('MPD');
    if (job.auto_play) opts.push('auto-play');
    if (job.video) opts.push('video');
    tr.appendChild(cell(opts.join(', ')));

    const statusCell = document.createElement('td');
    statusCell.className = 'status-' + job.status;
    statusCell.textContent = job.status;
    if (job.error) {
      statusCell.title = job.error;
    }
    tr.appendChild(statusCell);

    const filesCell = document.createElement('td');
    filesCell.className = 'mono';
    if (job.files && job.files.length) {
      filesCell.textContent = job.files.join(', ');
    }
    tr.appendChild(filesCell);

    const logCell = document.createElement('td');
    const link = document.createElement('a');
    link.href = '/log/' + encodeURIComponent(job.id);
    link.textContent = 'log';
    logCell.appendChild(link);
    tr.appendChild(logCell);

    tbody.appendChild(tr);
  }
}

function cell(text, className) {
  const td = document.createElement('td');
  if (className) td.className = className;
  td.textContent = text;
  return td;
}

loadJobs();
setInterval(loadJobs, 2000);
