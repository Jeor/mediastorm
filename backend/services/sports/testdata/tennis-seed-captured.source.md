Captured 2026-09-26 from the public ESPN tennis scoreboard:
https://site.api.espn.com/apis/site/v2/sports/tennis/wta/scoreboard?dates=20260912

This reduced response preserves one Guadalajara match containing Iva Jovic's competitor-level curatedRank.current = 2. The organizer independently describes her as the second seed (not her world ranking):
https://guadalajaraopen.com/iva-jovic-es-bicampeona-del-abierto-de-guadalajara/

Only the event identity and one competition are retained. Tournament seeds are mapped to a separate seed field only for tennis; missing or invalid seeds are omitted. Other tournament/match dates can be present in ESPN's date query, so do not treat the query date as each match's actual date.
