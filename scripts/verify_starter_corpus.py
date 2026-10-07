#!/usr/bin/env python3
"""Verify downloaded starter assets offline; Python standard library only.
This is a corpus preparation utility, not an application backend.
"""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CORPUS = ROOT / 'data/starter-corpus'


def sha256(path):
    checksum = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            checksum.update(block)
    return checksum.hexdigest()


def main():
    failures = []
    manifest = json.loads((CORPUS / 'manifest.json').read_text())
    for line in (CORPUS / 'SHA256SUMS').read_text().splitlines():
        expected, relative = line.split('  ', 1)
        path = (CORPUS / relative).resolve()
        if not path.is_relative_to(CORPUS.resolve()):
            failures.append(f'Unsafe checksum path: {relative}')
        elif not path.is_file() or sha256(path) != expected:
            failures.append(f'Missing or changed file: {relative}')
    for source in manifest['sources']:
        for asset in source['assets']:
            path = (ROOT / asset['local_path']).resolve()
            if not path.is_relative_to(CORPUS.resolve()):
                failures.append(f'Unsafe asset path: {path}')
                continue
            if not path.is_file():
                failures.append(f'Missing asset: {path.name}')
                continue
            if path.stat().st_size != asset['bytes'] or sha256(path) != asset['sha256']:
                failures.append(f'Asset integrity mismatch: {path.name}')
            if asset['kind'] == 'pdf':
                with path.open('rb') as stream:
                    if stream.read(5) != b'%PDF-':
                        failures.append(f'Not a PDF: {path.name}')
        page_map = json.loads((ROOT / source['page_map_path']).read_text())['pages']
        if len(page_map) != source['scan_page_count']:
            failures.append(f'Page count mismatch: {source["source_key"]}')
        if source['pdf_page_count'] != len(page_map) + 1:
            failures.append(f'Cover-sheet count mismatch: {source["source_key"]}')
        for i, page in enumerate(page_map):
            if page['scan_page_index'] != i or page['pdf_page_index'] != i + 1:
                failures.append(f'Invalid page mapping: {source["source_key"]}, {i}')
        if source['publication_status'] != 'not_approved' or source['processing']['auto_publish']:
            failures.append(f'Unexpected publication approval: {source["source_key"]}')
    if failures:
        raise SystemExit('\n'.join(failures))
    print(f'PASS: {len(manifest["sources"])} sources; asset checksums, sizes, PDF signatures and page-map structure verified.')
    print('This checks integrity, not OCR accuracy or publication approval.')


if __name__ == '__main__':
    main()
