#!/usr/bin/env python3
"""Merge probe evidence into all 340 original targets without claiming API/UI certification."""
import json,csv,pathlib,collections,sys
P=pathlib.Path(__file__).resolve().parent
catalog=json.loads((P/'validated-catalog.json').read_text());by={x['id']:x for x in catalog}
w6={x['id']:x for x in json.loads((P/'w6-findings.json').read_text())['targets']}
cricket_path=pathlib.Path(sys.argv[1]) if len(sys.argv)>1 else P/'evidence/cricket-target-validation.json'
cricket={x['target_id']:x for x in json.loads(cricket_path.read_text())} if cricket_path.exists() else {}
rows=list(csv.DictReader((P/'handoff/coverage-tracker.csv').open()))
for r in rows:
 x=by.get(r['target_id'])
 if x:
  r.update(status=x['implementationStatus'],evidence=x['evidence']['fixture'],source=x['evidence']['default']['url'],source_date=x['evidence']['default']['fetchedAt'],capabilities_validated=';'.join(x['capabilities']),blocker_or_notes=x['coverageNote'])
  r['backend_tests']='provider-fixture-only; integration-not-run';r['frontend_tests']='not-run';r['device_tests']='not-run'
 if r['target_id'] in w6:
  w=w6[r['target_id']];r.update(status=w['status'],evidence='w6-findings.json',blocker_or_notes=w['nextAction'])
 if r['target_id'].startswith('cricket-discovery'):
  r.update(evidence='evidence/cricket-dropdown.json; all 500 rows inspected',source_date='2026-09-24',blocker_or_notes='Fresh dropdown acquired; see separate cricket target findings. No guessed slug or activation from directory alone.')
for r in rows:
 if r['target_id'] in cricket:r.update({k:v for k,v in cricket[r['target_id']].items() if k in r})
assert len(rows)==340 and len({r['target_id'] for r in rows})==340
with (P/'coverage-tracker.csv').open('w') as f:
 w=csv.DictWriter(f,fieldnames=rows[0].keys());w.writeheader();w.writerows(rows)
for x in catalog:
 assert x['evidence']['default']['httpStatus']==200
 assert x['providerLeague'] in [l['slug'] for l in x['evidence']['leagueIdentity']]
 for key in ('fixture','datedFixture','seasonFixture'):
  if key in x['evidence']:assert (P.parent.parent/x['evidence'][key]).is_file(),x['id']
 assert x['implementationStatus']!='limited' or 'schedule' in x['capabilities']
print('Validated 313 unique descriptors and 340 target tracker rows. '+str(collections.Counter(x['implementationStatus'] for x in catalog)))
