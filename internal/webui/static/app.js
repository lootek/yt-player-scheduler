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
    tr.appendChild(linkCell(job.url, job.url));

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

    tr.appendChild(filesCell(job));

    const logCell = document.createElement('td');
    const link = document.createElement('a');
    link.href = '/log/' + encodeURIComponent(job.id);
    link.textContent = 'log';
    logCell.appendChild(link);
    tr.appendChild(logCell);

    tbody.appendChild(tr);
  }
}

function filesCell(job) {
  const td = document.createElement('td');
  td.className = 'mono';
  const paths = job.files && job.files.length ? job.files : (job.pending_files || []);
  const label = job.files && job.files.length ? '' : 'pending: ';
  if (paths.length) {
    td.textContent = label + paths.join(', ');
    td.title = paths.join('\n');
  }
  return td;
}

function cell(text, className) {
  const td = document.createElement('td');
  if (className) td.className = className;
  td.textContent = text;
  return td;
}

function linkCell(text, href) {
  const td = document.createElement('td');
  const a = document.createElement('a');
  a.href = href;
  a.textContent = text;
  a.target = '_blank';
  a.rel = 'noopener noreferrer';
  td.appendChild(a);
  return td;
}

loadJobs();
setInterval(loadJobs, 2000);
