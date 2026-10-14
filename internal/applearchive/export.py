"""Read Apple Notes MCP exports or VoiceMemo robot SQLite; never mutate sources."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import sqlite3
import sys


def timestamp(value):
    parsed = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
    if parsed.tzinfo is None:
        raise ValueError('timestamp requires timezone')
    return int(parsed.timestamp() * 1000)


def document(source, native_id, text, date, metadata, locator, raw):
    if not isinstance(native_id, str) or not native_id.strip():
        raise ValueError('native ID required')
    identity = json.dumps([source, native_id], ensure_ascii=False, separators=(',', ':'))
    digest = hashlib.sha256(json.dumps(raw, sort_keys=True, ensure_ascii=False, default=str).encode()).hexdigest()
    metadata.update(native_id=native_id, provenance={'locator': locator, 'record_sha256': digest})
    return dict(id=hashlib.sha256(identity.encode()).hexdigest(), source=source,
                type='note' if source == 'apple_notes' else 'transcript',
                text=text.strip(), timestamp_unix_ms=timestamp(date), metadata=metadata)


def notes(path):
    files = sorted(path.glob('*.json')) if path.is_dir() else [path]
    if not files:
        raise ValueError('no JSON exports')
    docs, skipped, seen, pages = [], {}, set(), []
    for file in files:
        raw = file.read_bytes()
        data = json.loads(raw)
        data = data.get('structuredContent', data)
        if not isinstance(data.get('accounts'), list):
            raise ValueError('expected export-notes-json accounts array')
        if data.get('page') is not None:
            page = data['page']
            actual = sum(len(folder['notes']) for account in data['accounts'] for folder in account['folders'])
            if any(type(page.get(k)) is not int or page[k] < 0 for k in ('offset', 'returned', 'totalAvailable')) or type(page.get('hasMore')) is not bool:
                raise ValueError('invalid export pagination metadata')
            if page['returned'] != actual:
                raise ValueError('page returned count differs from records')
            pages.append(page)
        for account in data['accounts']:
            for folder in account['folders']:
                for note in folder['notes']:
                    reason = ('locked' if note.get('passwordProtected') else
                              'content_omitted' if note.get('contentOmitted') else
                              'empty' if not note.get('plaintext', '').strip() else None)
                    if reason:
                        skipped[reason] = skipped.get(reason, 0) + 1
                        continue
                    timestamp(note['created'])
                    meta = {k: note.get(k) for k in ('title', 'created', 'modified', 'shared', 'strippedImages')}
                    meta.update(account=account['name'], folder=folder['name'], export_date=data.get('exportDate'),
                                archive_sha256=hashlib.sha256(raw).hexdigest())
                    doc = document('apple_notes', note['id'], note['plaintext'], note['modified'], meta, str(file.resolve()), note)
                    if doc['id'] in seen:
                        raise ValueError('duplicate note ID across exports; use one export snapshot')
                    seen.add(doc['id'])
                    docs.append(doc)
    # A directory must contain a contiguous, complete set of one export's pages.
    if pages:
        if len(pages) != len(files):
            raise ValueError('cannot mix paged and unpaged exports')
        pages.sort(key=lambda p: p['offset'])
        expected, total = 0, pages[0]['totalAvailable']
        for i, page in enumerate(pages):
            if page['offset'] != expected or page['totalAvailable'] != total:
                raise ValueError('missing/overlapping pages or changing export size')
            expected += page['returned']
            if page['hasMore'] != (i < len(pages) - 1) or (page['hasMore'] and page.get('nextOffset') != expected):
                raise ValueError('incomplete export pagination; collect all pages')
        if expected != total:
            raise ValueError('export page count does not match totalAvailable')
    return docs, dict(skipped=skipped, export_pages=len(files), coverage='export snapshot; attachments not indexed')


def voice(path):
    docs, skipped = [], {}
    source_records, latest_recorded_at = 0, None
    with sqlite3.connect(path.resolve().as_uri() + '?mode=ro', uri=True) as conn:
        conn.row_factory = sqlite3.Row
        rows = conn.execute('''SELECT m.*, t.transcript_id, t.transcript_text, t.transcript_source,
            t.transcript_status, t.transcript_version, t.generated_at, t.needs_refresh
            FROM memos m LEFT JOIN transcripts t ON t.transcript_id = (
                SELECT transcript_id FROM transcripts WHERE memo_id=m.memo_id
                ORDER BY generated_at DESC, transcript_id DESC LIMIT 1)
            ORDER BY m.created_at, m.memo_id''')
        for row in rows:
            row = dict(row)
            source_records += 1
            if latest_recorded_at is None or timestamp(row['created_at']) > timestamp(latest_recorded_at):
                latest_recorded_at = row['created_at']
            reason = ('stale' if row['needs_refresh'] else
                      row['transcript_status'] or 'missing')
            if reason != 'success' or not (row['transcript_text'] or '').strip():
                reason = 'empty' if reason == 'success' else reason
                skipped[reason] = skipped.get(reason, 0) + 1
                continue
            meta = {k: row[k] for k in ('memo_id', 'source_unique_id', 'resolved_audio_path', 'created_at',
                    'duration_seconds', 'transcript_id', 'transcript_source', 'transcript_version', 'generated_at')}
            meta['title'] = row['source_label'] or row['manifest_title'] or ''
            docs.append(document('voicememo', row['source_unique_id'], row['transcript_text'], row['created_at'],
                                 meta, str(path.resolve()) + '#memos/' + row['memo_id'], row))
    return docs, dict(skipped=skipped, source_records=source_records, latest_recorded_at=latest_recorded_at, coverage='cached robot DB; not live-store completeness')


def load(source, path):
    docs, report = (notes if source == 'apple_notes' else voice)(Path(path).expanduser())
    return dict(documents=docs, report=dict(report, indexed_candidates=len(docs)))


if __name__ == '__main__':
    try:
        json.dump(load(sys.argv[1], sys.argv[2]), sys.stdout, ensure_ascii=False)
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
