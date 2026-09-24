#!/usr/bin/env python3
"""Second-pass bounded season/date verification; no retirement inference from stale data."""
from probe_coverage import *
import concurrent.futures as cf
rows=json.loads((ROOT/'validated-catalog.json').read_text())
def refine(x):
 if x['implementationStatus']!='candidate':return x
 url=x['evidence']['default']['url'];events=x['evidence']['events']
 # A UTC early-morning event can belong to the previous US scoreboard day.
 day=x['evidence']['testedDate']
 if events and day[:4] in ('2026','2027') and not x['evidence']['dateVerified']:
  day=(dt.datetime.strptime(day,'%Y%m%d')-dt.timedelta(days=1)).strftime('%Y%m%d');u=url+'?dates='+day
 else:u=url+'?dates=20260101-20260924&limit=25'
 rec,j=get(u);es=j.get('events',[]);stem=x['providerSport']+'--'+x['providerLeague']+'--season';save(OUT/(stem+'.json'),compact(j));x['evidence']['seasonProbe']=rec;x['evidence']['seasonFixture']='docs/sports-coverage/evidence/'+stem+'.json'
 valid=[]
 for e in es:
  ps=[p for c in e.get('competitions',[]) for p in c.get('competitors',[])]
  named=[p for p in ps if any(p.get(k,{}).get('displayName') or p.get(k,{}).get('name') for k in ('team','athlete'))]
  if e.get('id') and e.get('name') and e.get('date','')[:4] in ('2026','2027') and named:valid.append(e)
 if valid:
  x['implementationStatus']='limited';x['capabilities']=['schedule'];x['coverageNote']='Dated current/relevant-season named-participant sample verified; live transitions and season completeness unverified.'
  x['evidence']['validatedEvents']=[{'id':e['id'],'date':e['date'],'name':e['name'],'season':e.get('season'),'status':e.get('status')} for e in valid[:3]]
  if any(e.get('status',{}).get('type',{}).get('completed') and any('score' in p for c in e.get('competitions',[]) for p in c.get('competitors',[])) for e in valid):x['capabilities'].append('final-result')
 save(OUT/(x['providerSport']+'--'+x['providerLeague']+'--evidence.json'),x)
 return x
with cf.ThreadPoolExecutor(max_workers=2) as ex:rows=list(ex.map(refine,rows))
save(ROOT/'validated-catalog.json',rows)
from collections import Counter
print(Counter(x['implementationStatus'] for x in rows))
