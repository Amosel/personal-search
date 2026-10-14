import importlib.util
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('exporter', Path(__file__).with_name('export.py'))
exporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exporter)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)
        self.note = dict(id='native-id', title='Title', plaintext='Body', created='2026-01-01T00:00:00Z', modified='2026-02-01T00:00:00Z')

    def page(self, notes, **page):
        return dict(accounts=[dict(name='iCloud', folders=[dict(name='Notes', notes=notes)])],
                    page=dict(offset=0, returned=len(notes), totalAvailable=len(notes), hasMore=False, **page))

    def write(self, data, name='page.json'):
        path = self.path / name
        path.write_text(json.dumps(data))
        return path

    def test_native_identity_survives_title_text_and_folder_changes(self):
        path = self.write(self.page([self.note]))
        first = exporter.load('apple_notes', path)['documents'][0]
        self.note.update(title='Renamed', plaintext='Changed')
        page = self.page([self.note]); page['accounts'][0]['folders'][0]['name'] = 'Moved'
        second = exporter.load('apple_notes', self.write(page))['documents'][0]
        self.assertEqual(first['id'], second['id'])
        self.assertNotEqual(first['metadata']['provenance']['record_sha256'], second['metadata']['provenance']['record_sha256'])
        self.assertEqual(second['timestamp_unix_ms'], 1769904000000)
        self.assertEqual(second['metadata']['folder'], 'Moved')

    def test_pagination_and_skips(self):
        first = self.page([self.note]); first['page'].update(hasMore=True, nextOffset=1, totalAvailable=2)
        self.write(first)
        with self.assertRaisesRegex(ValueError, 'pagination'):
            exporter.load('apple_notes', self.path)
        second = self.page([dict(self.note, id='locked', passwordProtected=True)])
        second['page'].update(offset=1, totalAvailable=2)
        self.write(second, 'page2.json')
        result = exporter.load('apple_notes', self.path)
        self.assertEqual(len(result['documents']), 1)
        self.assertEqual(result['report']['skipped'], {'locked': 1})
        second['page']['offset'] = 2
        self.write(second, 'page2.json')
        with self.assertRaisesRegex(ValueError, 'pages'):
            exporter.load('apple_notes', self.path)

    def test_false_page_counts_and_mixed_formats_fail(self):
        page = self.page([self.note]); page['page']['returned'] = 0
        with self.assertRaisesRegex(ValueError, 'count differs'):
            exporter.load('apple_notes', self.write(page))
        self.write(self.page([self.note]))
        legacy = self.page([dict(self.note, id='other')]); del legacy['page']
        self.write(legacy, 'legacy.json')
        with self.assertRaisesRegex(ValueError, 'mix paged'):
            exporter.load('apple_notes', self.path)

    def test_missing_ids_dates_and_duplicate_notes_fail(self):
        for field, value in [('id', ''), ('modified', 'not-a-date'), ('modified', '2026-01-01T00:00:00')]:
            with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                exporter.load('apple_notes', self.write(self.page([dict(self.note, **{field: value})])))
        with self.assertRaisesRegex(ValueError, 'duplicate'):
            exporter.load('apple_notes', self.write(self.page([self.note, self.note])))

    def test_latest_transcript_and_missing_stale_reporting_readonly(self):
        db = self.path / 'voice.db'
        with sqlite3.connect(db) as c:
            c.executescript('''CREATE TABLE memos(memo_id TEXT, source_unique_id TEXT, created_at TEXT,
                duration_seconds REAL, resolved_audio_path TEXT, source_label TEXT, manifest_title TEXT);
                CREATE TABLE transcripts(transcript_id TEXT, memo_id TEXT, transcript_text TEXT,
                transcript_source TEXT, transcript_status TEXT, transcript_version TEXT, generated_at TEXT, needs_refresh INT);''')
            for i in range(3):
                c.execute('INSERT INTO memos VALUES(?,?,?,30,?,?,NULL)', (str(i), 'native'+str(i), '2026-01-01T00:00:00Z', '/audio/'+str(i), 'title'))
            for tid, mid, generated, status, refresh in [('old','0','2026-01-01','success',0), ('new','0','2026-02-01','success',0), ('stale','1','2026-02-01','success',1)]:
                c.execute('INSERT INTO transcripts VALUES(?,?,?,?,?,?,?,?)', (tid,mid,tid,'apple',status,'v1',generated,refresh))
        before = db.read_bytes()
        result = exporter.load('voicememo', db)
        self.assertEqual(db.read_bytes(), before)
        self.assertEqual([d['text'] for d in result['documents']], ['new'])
        self.assertEqual(result['report']['skipped'], {'stale': 1, 'missing': 1})
        self.assertEqual(result['documents'][0]['timestamp_unix_ms'],1767225600000)
        with self.assertRaises(sqlite3.OperationalError):
            exporter.load('voicememo', self.path/'absent.db')
        self.assertFalse((self.path/'absent.db').exists())

if __name__ == '__main__':
    unittest.main()
