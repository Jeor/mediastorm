#!/usr/bin/env python3
"""Evidence-backed history dispositions; never deduplicate merely similar names."""
import json,pathlib,collections
P=pathlib.Path(__file__).resolve().parent
r=json.loads((P/'validated-catalog.json').read_text())
refs={
'olympic':('https://la28.org/en/games-plan/olympics.html','LA28 official program includes baseball, basketball, golf, rugby sevens and football in the 2028 summer edition. Existing provider samples are historical editions, not retired sports.'),
'wec':('https://www.ufc.com/news/wec-53-numbers','UFC identifies December 16, 2010 as the final WEC event; matches provider final date.'),
'strikeforce':('https://www.ufc.com/news/saffiedine-steals-show-strikeforce-main-card-results','UFC identifies January 12, 2013 as the last Strikeforce event; matches provider final date.'),
'pride':('https://www.ufc.com/news/machidarampage-defining-moments','UFC retrospective establishes PRIDE had ended by 2007; provider final sample is April 2007.'),
'xfl':('https://www.theufl.com/news/united-football-league-ufl-set-to-launch-as-the-premier-spring-football-league','Official UFL announcement establishes XFL/USFL merger into UFL for 2024. XFL2023 records remain distinct historical identity; do not merge event IDs.'),
'hockey-world-cup':('https://www.nhl.com/news/world-cup-of-hockey-returning-in-2028-nhl-nhlpa-announce','NHL confirms return in 2028; historical2016 feed is dormant cycle, not retirement.'),
'uefa.euro':('https://www.uefa.com/euro2028/news/028f-1b599fe02d18-0c758142f5e4-1000/','UEFA identifies next tournament in2028 and qualifying in2027; returned2024 edition is historical cycle.'),
'uefa.euroq':('https://www.uefa.com/euro2028/news/028f-1b599fe02d18-0c758142f5e4-1000/','UEFA identifies next qualification in2027; returned2024 qualifying edition is historical cycle.'),
'uefa.weuro':('https://www.uefa.com/womenseuro/news/029b-1e56a4b43051-3ec6381b01f6-1000/','Official UEFA2025 final reference matches dated historical provider final. No2026 schedule asserted.'),
'289237':('https://www.world.rugby/media-zone/advisory/1016471?lang=en','World Rugby identifies2025 final edition; archived2025 provider tournament is not a current2026 live schedule.')}
findings=[]
for x in r:
 if x['implementationStatus'] not in ('candidate','archival'):continue
 e=x['evidence'];sport=x['providerSport'];slug=x['providerLeague']
 if sport=='golf' and slug=='ntw' and e['dateVerified'] and e['events'] and e['events'][0].get('date','').startswith('2026'):
  x.update(implementationStatus='limited',capabilities=['schedule'],coverageNote='Dated2026 named tournament schedule and competition identity validated. No athlete participants returned; schedule-only tournament card, no leaderboard/results claim.')
  (P/'evidence/golf--ntw--evidence.json').write_text(json.dumps(x,separators=(',',':'))+'\n')
  continue
 first=(e['events'] or [{}])[0];last=first.get('date','none');seasonp=P.parent.parent/e['seasonFixture'] if e.get('seasonFixture') else None
 season=json.loads(seasonp.read_text()) if seasonp else {};es=season.get('events',[])
 named=sum(1 for ev in es for c in ev.get('competitions',[]) for p in c.get('competitors',[]) if p.get('team',p.get('athlete',{})).get('displayName'))
 k='olympic' if ('olympics' in slug or (sport=='rugby' and slug in ('282','283'))) else slug
 finding={'id':x['id'],'providerDefaultEventDate':last,'datedMatch':e['dateVerified'],'followupURL':e.get('seasonProbe',{}).get('url'),'followupHTTP':e.get('seasonProbe',{}).get('httpStatus'),'followupEventCount':len(es),'followupNamedParticipants':named,'assessedAt':'2026-09-24'}
 if k in refs:
  url,note=refs[k];x['implementationStatus']='archival';x['capabilities']=[];x['coverageNote']=note+' Historical fixture retained; excluded from live refresh until a future edition is independently validated.';e['officialReference']={'url':url,'checkedAt':'2026-09-24','observation':note};finding.update(disposition='archival',officialReference=e['officialReference'],nextAction='Retain historical fixtures without live polling; revalidate distinct future edition if applicable.')
 else:
  finding['disposition']='candidate'
  if last=='none':reason='Default scoreboard contains no events; current-year query supplies no usable dated named-participant sample.'
  elif last[:4]=='2026':reason='Current-season event exists, but no named participant contract was returned in the tested default/dated/season payloads.'
  else:reason=f'Default sample is dated {last}; tested2026 query yields {len(es)} events and {named} named participant records, without a usable2026 sample. Competition retirement or alias identity is not established.'
  finding['reason']=reason;finding['nextAction']='Resolve current official competition calendar and an authorized source; if cyclical, corroborate edition before archival/history classification. Do not poll this candidate as current.';x['coverageNote']=reason+' '+finding['nextAction']
 findings.append(finding)
 (P/'evidence'/(sport+'--'+slug+'--evidence.json')).write_text(json.dumps(x,separators=(',',':'))+'\n')
(P/'validated-catalog.json').write_text(json.dumps(r,separators=(',',':'))+'\n')
(P/'source-dispositions.json').write_text(json.dumps(findings,indent=2)+'\n')
print(collections.Counter(x['implementationStatus'] for x in r))
