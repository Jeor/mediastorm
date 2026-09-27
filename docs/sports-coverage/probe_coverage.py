#!/usr/bin/env python3
"""Bounded public ESPN evidence capture. No credentials or recursive refs. Max 4 workers."""
import concurrent.futures as cf, datetime as dt, json, pathlib, time, urllib.request, urllib.error, hashlib
ROOT=pathlib.Path(__file__).resolve().parent
OUT=ROOT/'evidence'; OUT.mkdir(exist_ok=True)
TODAY='2026-09-24'
def save(p,obj):p.write_text(json.dumps(obj,ensure_ascii=False,separators=(',',':'))+'\n')
def get(url):
    at=dt.datetime.now(dt.timezone.utc).isoformat(); rec={'url':url,'fetchedAt':at}
    time.sleep(.3)
    try:
        with urllib.request.urlopen(url,timeout=25) as r:
            data=r.read(16*1024*1024+1);rec.update(httpStatus=r.status,sha256=hashlib.sha256(data).hexdigest(),bytes=len(data))
            if len(data)>16*1024*1024:raise ValueError('response exceeds 16 MiB')
            return rec,json.loads(data)
    except urllib.error.HTTPError as e:
        rec.update(httpStatus=e.code,error=str(e),retryAfter=e.headers.get('Retry-After'))
        if e.code==429:time.sleep(min(60,int(e.headers.get('Retry-After','30')) if e.headers.get('Retry-After','30').isdigit() else 30))
    except Exception as e:rec.update(error=str(e))
    return rec,{}
def compact(j):return {k:j[k] for k in ('leagues','season','day','calendar','events') if k in j}
def probe(row):
    sport,slug=row['sport'],row['espn_slug'];stem=sport+'--'+slug
    rec,j=get(row['scoreboard_url']);events=j.get('events',[])
    chosen=next((e for e in events if e.get('date','')[:4]=='2026'),events[0] if events else {})
    day=chosen.get('date','')[:10].replace('-','') or TODAY.replace('-','')
    dated,d=get(row['scoreboard_url']+'?dates='+day)
    # Retain exact provider content, including all nested competitions, in three representative events.
    fixture=compact(j);fixture['events']=events[:3]
    if chosen and chosen not in fixture['events']:fixture['events'].append(chosen)
    save(OUT/(stem+'.json'),fixture)
    datedfixture=compact(d);datedfixture['events']=d.get('events',[])[:3];save(OUT/(stem+'--dated.json'),datedfixture)
    usable=[]
    for e in events:
        comps=e.get('competitions',[]);ps=[p for c in comps for p in c.get('competitors',[])]
        named=[p for p in ps if any(p.get(k,{}).get('displayName') or p.get(k,{}).get('name') for k in ('team','athlete'))]
        if e.get('id') and e.get('name') and e.get('date','')[:4] in ('2026','2027') and named:usable.append(e)
    dateverified=bool(chosen.get('id') and any(e.get('id')==chosen['id'] for e in d.get('events',[])))
    status='limited' if usable and dateverified else 'candidate'
    caps=['schedule'] if status=='limited' else []
    if status=='limited' and any(e.get('status',{}).get('type',{}).get('completed') and any('score' in p for c in e.get('competitions',[]) for p in c.get('competitors',[])) for e in usable):caps.append('final-result')
    if status=='limited' and any(any(p.get('linescores') for c in e.get('competitions',[]) for p in c.get('competitors',[])) for e in usable):caps.append('period-detail')
    app={'football':'american-football','hockey':'ice-hockey','rugby':'rugby-union','racing':'motorsport'}.get(sport,sport)
    if slug in ('college-softball','lls'):app='softball'
    note='Dated current/relevant-season named-participant schedule sample verified; completeness and live transitions unverified.' if status=='limited' else 'No independently verified dated current/relevant-season named-participant sample; historical/empty feed is not proof of retirement.'
    desc={'id':row['proposed_new_id'],'name':row['league_name'],'sport':app,'category':app,'eventKind':row['adapter_family'],'supportsTeams':row['adapter_family']=='team-match','enabled':False,'provider':'espn','providerSport':sport,'providerLeague':slug,'applicationSport':app,'college':str(row['is_college']).lower()=='true','adapter':row['adapter_family'],'implementationStatus':status,'capabilities':caps,'coverageNote':note,'evidence':{'default':rec,'dated':dated,'testedDate':day,'dateVerified':dateverified,'fixture':'docs/sports-coverage/evidence/'+stem+'.json','datedFixture':'docs/sports-coverage/evidence/'+stem+'--dated.json','eventCount':len(events),'season':j.get('season'),'leagueIdentity':[{k:l.get(k) for k in ('id','name','slug','season')} for l in j.get('leagues',[])],'events':[{'id':e.get('id'),'date':e.get('date'),'name':e.get('name'),'season':e.get('season'),'status':e.get('status'),'participants':[{'id':p.get('id'),'type':p.get('type'),'name':p.get('team',p.get('athlete',{})).get('displayName'),'score':p.get('score')} for c in e.get('competitions',[]) for p in c.get('competitors',[])]} for e in fixture['events']]}}
    save(OUT/(stem+'--evidence.json'),desc)
    return desc
if __name__=='__main__':
    rows=json.loads((ROOT/'handoff/missing-leagues.json').read_text())
    with cf.ThreadPoolExecutor(max_workers=4) as ex: result=list(ex.map(probe,rows))
    save(ROOT/'validated-catalog.json',result)
    from collections import Counter
    print(Counter(r['implementationStatus'] for r in result),flush=True)
