#!/usr/bin/env python3
"""Optional capability evidence. <=4 provider workers; one request per exact endpoint."""
import pathlib,json,concurrent.futures as cf,sys,collections,re
sys.path.insert(0,str(pathlib.Path(__file__).parent))
from probe_coverage import get,save
P=pathlib.Path(__file__).resolve().parent;OUT=P/'evidence'/'optional';OUT.mkdir(exist_ok=True)
rows=[x for x in json.loads((P/'validated-catalog.json').read_text()) if x['implementationStatus']=='limited' and x['provider']=='espn']
manifest={x['proposed_new_id']:x for x in json.loads((P/'handoff/missing-leagues.json').read_text())}
def pagination(j):
 keys=('count','total','totalCount','pageCount','pageIndex','pageSize','limit','offset','next','nextPage')
 return {k:j[k] for k in keys if k in j}
def walk(o):
 if isinstance(o,dict):
  yield o
  for v in o.values():yield from walk(v)
 elif isinstance(o,list):
  for v in o:yield from walk(v)
def probe(x):
 sport=x['providerSport'];slug=x['providerLeague'];stem=sport+'--'+slug;root='https://site.api.espn.com/apis/site/v2/sports/'+sport+'/'+slug
 out={'id':x['id'],'capabilities':[]}
 rec,j=get(root+'/teams?limit=1000');leagues=[l for s in j.get('sports',[]) for l in s.get('leagues',[])];league=next((l for l in leagues if l.get('slug')==slug),{});teams=[t.get('team',{}) for t in league.get('teams',[])];named=[t for t in teams if t.get('id') and t.get('displayName')];ids={str(t['id']) for t in named};meta={'response':pagination(j),'league':pagination(league)};totals=[o.get(k) for o in (j,league) for k in ('totalCount','total','count') if isinstance(o.get(k),int)];hasnext=any(o.get(k) for o in (j,league) for k in ('next','nextPage'));complete=bool(totals and max(totals)==len(ids) and len(ids)==len(teams) and len(ids)<1000 and not hasnext)
 # Preserve provider shape while avoiding repeated giant logo arrays in fixture.
 fixture={'sports':[{'id':s.get('id'),'leagues':[{**{k:v for k,v in l.items() if k!='teams'},'teams':[{'team':{k:v for k,v in t.get('team',{}).items() if k not in ('logos','links')}} for t in l.get('teams',[])]} for l in s.get('leagues',[])]} for s in j.get('sports',[])]};save(OUT/(stem+'--teams.json'),fixture)
 out['teams']={'request':rec,'fixture':'docs/sports-coverage/evidence/optional/'+stem+'--teams.json','leagueIdentity':{k:league.get(k) for k in ('id','slug','name','season')},'returnedCount':len(teams),'namedUniqueCount':len(ids),'pagination':meta,'requestedLimit':1000,'complete':complete,'reason':'Explicit count/pagination proves complete named identity set.' if complete else 'No explicit total/pagination proves completeness, or response is empty/truncated; requested limit and HTTP200 are insufficient.'}
 if complete:out['capabilities'].append('team-catalog')
 rec,j=get('https://site.api.espn.com/apis/v2/sports/'+sport+'/'+slug+'/standings?season=2026');tables=[]
 for o in walk(j):
  if isinstance(o.get('entries'),list):
   namedrows=[t for t in o['entries'] if t.get('team',{}).get('id') and t.get('team',{}).get('displayName') and t.get('stats')]
   tables.append({'season':o.get('season'),'seasonType':o.get('seasonType'),'name':o.get('name'),'rowCount':len(o['entries']),'namedStatRowCount':len(namedrows)})
 usable=bool(tables and sum(t['namedStatRowCount'] for t in tables)>0 and all(t['season']==2026 for t in tables if t['namedStatRowCount']))
 expectedLeagueID=next((str(l['id']) for l in x['evidence']['leagueIdentity'] if l.get('id')),None);uid=j.get('uid','');identity=bool(expectedLeagueID and '~l:'+expectedLeagueID+'~' in uid+'~');usable=usable and identity
 # Raw provider-shaped standings retained; contains rows/season needed for validation.
 save(OUT/(stem+'--standings.json'),j)
 out['standings']={'request':rec,'fixture':'docs/sports-coverage/evidence/optional/'+stem+'--standings.json','identityVerified':identity,'expectedLeagueID':expectedLeagueID,'returnedUID':uid,'returnedName':j.get('name'),'tables':tables,'usable':usable,'reason':'Named statistical rows match league and2026 season.' if usable else 'No identity-matching named statistical table for requested2026 season; endpoint success alone is insufficient.'}
 if usable:out['capabilities'].append('standings')
 m=manifest[x['id']]
 if m.get('summary_probe_http') not in ('not-tested',None):
  ev=next((e for e in x['evidence']['events'] if e.get('date','').startswith('2026')),None)
  if ev:
   rec,j=get(root+'/summary?event='+str(ev['id']));h=j.get('header',{});identity=str(h.get('id',''))==str(ev['id']);participants=[c for co in h.get('competitions',[]) for c in co.get('competitors',[])];usable=identity and all(p.get('team',p.get('athlete',{})).get('displayName') for p in participants) and bool(participants) and any(j.get(k) for k in ('gameInfo','boxscore','plays','rosters','leaders','standings'))
   save(OUT/(stem+'--summary.json'),j);out['summary']={'request':rec,'fixture':'docs/sports-coverage/evidence/optional/'+stem+'--summary.json','requestedEventID':ev['id'],'returnedEventID':h.get('id'),'identityVerified':identity,'participantCount':len(participants),'nonemptySections':[k for k in ('gameInfo','boxscore','plays','rosters','leaders','standings') if j.get(k)],'usable':usable}
   if usable:out['capabilities'].append('summary')
 save(OUT/(stem+'--evidence.json'),out);return out
with cf.ThreadPoolExecutor(max_workers=4) as pool:result=list(pool.map(probe,rows))
save(P/'evidence/optional-capabilities.json',result)
print(len(result),collections.Counter(c for x in result for c in x['capabilities']))
