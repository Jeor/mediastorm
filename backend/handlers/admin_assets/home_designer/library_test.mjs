import assert from 'node:assert/strict';
import test from 'node:test';
import { expandStreamingService } from './library.js';
import { createStore } from './store.js';

const streamingEntry = () => ({
    type: 'streaming-service', catalogOnly: true,
    fields: [
        { path: 'service', options: [{ value: 'netflix', label: 'Netflix' }] },
        { path: 'media', options: [{ value: 'movies', label: 'Movies' }, { value: 'shows', label: 'TV shows' }, { value: 'both', label: 'Movies and TV shows' }] },
    ],
});

test('streaming expansion preserves the selected media choice with canonical apply-ready fields', () => {
    // Break caught: streamed rows using the wrong list property or silently ignoring a movies/shows/both selection.
    const entry = streamingEntry();
    const movies = expandStreamingService(entry, [], { service: 'netflix', media: 'movies' });
    const shows = expandStreamingService(entry, [], { service: 'netflix', media: 'shows' });
    const both = expandStreamingService(entry, [], { service: 'netflix', media: 'both' });
    assert.deepEqual(movies.map((row) => [row.id, row.listUrl]), [['streaming-service-movies', 'https://mdblist.com/lists/snoak/netflix-top-10-movies/json']]);
    assert.deepEqual(shows.map((row) => [row.id, row.listUrl]), [['streaming-service-shows', 'https://mdblist.com/lists/snoak/netflix-top-10-shows/json']]);
    assert.deepEqual(both.map((row) => row.id), ['streaming-service-movies', 'streaming-service-shows']);
    assert.ok(both.every((row) => row.type === 'mdblist' && typeof row.listUrl === 'string' && !Object.hasOwn(row, 'listURL')));
});

test('streaming expansion generates a distinct stable instance when prior rows collide', () => {
    // Break caught: a second configured streaming provider reusing the first provider's row IDs.
    const rows = [{ id: 'streaming-service-movies' }, { id: 'streaming-service-shows' }];
    const next = expandStreamingService(streamingEntry(), rows, { service: 'netflix', media: 'both' });
    assert.deepEqual(next.map((row) => row.id), ['streaming-service-2-movies', 'streaming-service-2-shows']);
});

test('every streaming media expansion is apply-valid against the MDBList catalog contract', () => {
    // Break caught: an apparently configured streaming choice producing a row that the local Apply gate rejects.
    for (const media of ['movies', 'shows', 'both']) {
        const rows = expandStreamingService(streamingEntry(), [], { service: 'netflix', media });
        const store = createStore({
            scope: { kind: 'global' }, revision: 'revision',
            rows: { inherited: false, effective: { shelves: [] } }, theme: { inherited: false, effective: {} },
            catalog: [{ type: 'mdblist', available: true, fields: [{ path: 'listUrl', label: 'MDBList URL', required: true, type: 'url' }] }],
        });
        rows.forEach((row) => store.dispatch({ type: 'rows/add', row }));
        assert.equal(store.isApplyValid(), true, media);
    }
});
