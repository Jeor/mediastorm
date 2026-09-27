import json,urllib.request,urllib.parse,concurrent.futures,datetime,hashlib
from pathlib import Path
rows=json.loads(Path('docs/sports-coverage/evidence/coverage-actual-api-all.json').read_text())['leagues']
now=datetime.datetime.now(datetime.timezone.utc);date=now.strftime('%Y%m%d')
def probe(row):
 sport,slug=row['providerSport'],row.get('providerLeague','')
 q={'limit':'200','dates':date}
 if row['id']=='college-football':q['groups']='90'
 elif row['id'] in ['mens-college-basketball','womens-college-basketball']:q['groups']='50'
 url=f'https://site.api.espn.com/apis/site/v2/sports/{sport}/{slug}/scoreboard'
 if row['eventKind']!='race':url+='?'+urllib.parse.urlencode(q)
 out={'id':row['id'],'url':url,'checkedAt':now.isoformat()}
 try:
  with urllib.request.urlopen(urllib.request.Request(url,headers={'Accept':'application/json'}),timeout=18) as r:b=r.read(16*1024*1024);out['httpStatus']=r.status
  d=json.loads(b);out.update(sha256=hashlib.sha256(b).hexdigest(),eventCount=len(d.get('events',[])),returnedLeagues=[{'id':v.get('id'),'slug':v.get('slug'),'name':v.get('name')} for v in d.get('leagues',[])],count=d.get('count'),pageCount=d.get('pageCount'))
  out['identityMatches']=any(x['slug']==slug for x in out['returnedLeagues'])
 except Exception as e:out['error']=str(e)
 return out
active=[r for r in rows if r['provider']=='espn' and r['providerSport']!='cycling' and r['id']!='motogp']
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:result=list(ex.map(probe,active))
p=Path('docs/sports-coverage/evidence/request-parameter-audit.json');p.write_text(json.dumps({'date':date,'results':result},indent=2)+'\n')
print(json.dumps({'requests':len(result),'http200':sum(r.get('httpStatus')==200 for r in result),'identityMatches':sum(r.get('identityMatches',False) for r in result),'issues':[r for r in result if r.get('error') or not r.get('identityMatches')]},indent=2))
