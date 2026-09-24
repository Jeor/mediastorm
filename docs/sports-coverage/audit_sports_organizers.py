import json,re,urllib.request,concurrent.futures,datetime,hashlib
from pathlib import Path
now=datetime.datetime.now(datetime.timezone.utc);year=now.year
urls=[('cfl-fixtures',f'https://api.stats.cfl.ca/fixtures/{year}'),('cfl-teams','https://api.stats.cfl.ca/teams'),('cfl-standings',f'https://api.stats.cfl.ca/standings/{year}'),('motogp-seasons','https://api.motogp.pulselive.com/motogp/v1/results/seasons'),('boxing',f'https://www.thesportsdb.com/api/v1/json/123/eventsday.php?d={now:%Y-%m-%d}&l=4445')]
s=Path('backend/services/sports/cycling.go').read_text()
for host in re.findall(r'"(racecenter\.[^"]+)"',s):urls.append((host,f'https://{host}/api/stage-{year}'))
def probe(pair):
 name,url=pair;out={'target':name,'url':url,'checkedAt':now.isoformat()}
 try:
  with urllib.request.urlopen(urllib.request.Request(url,headers={'Accept':'application/json'}),timeout=18) as r:b=r.read(8*1024*1024);out['httpStatus']=r.status
  d=json.loads(b);out.update(jsonType=type(d).__name__,sha256=hashlib.sha256(b).hexdigest(),items=len(d),sampleKeys=list(d)[:8] if isinstance(d,dict) else None)
  if name=='motogp-seasons':out['currentSeason']=next((v['id'] for v in d if v.get('year')==year),None)
 except Exception as e:out['error']=str(e)
 return out
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:rows=list(ex.map(probe,urls))
season=next((r.get('currentSeason') for r in rows if r['target']=='motogp-seasons'),None)
if season:
 for route in ['categories','events']:rows.append(probe(('motogp-'+route,f'https://api.motogp.pulselive.com/motogp/v1/results/{route}?seasonUuid={season}')))
Path('docs/sports-coverage/evidence/organizer-parameter-audit.json').write_text(json.dumps(rows,indent=2)+'\n')
print(json.dumps({'requests':len(rows),'http200':sum(x.get('httpStatus')==200 for x in rows),'issues':[x for x in rows if x.get('error')]},indent=2))
