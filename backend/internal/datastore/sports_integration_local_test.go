package datastore

import (
 "context"
 "net/url"
 "os"
 "testing"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/jackc/pgx/v5/stdlib"
 "github.com/pressly/goose/v3"
)

func TestSportsIntegrationMigrationPaths(t *testing.T) {
 dsn := os.Getenv("SPORTS_INTEGRATION_DB_URL")
 if dsn == "" { t.Skip("local integration database required") }
 ctx := context.Background()
 admin,err:=pgxpool.New(ctx,dsn);if err!=nil {t.Fatal(err)};defer admin.Close()
 for _, upstream:=range []bool{false,true} {
  name:="sports_integration_fresh";if upstream {name="sports_integration_upstream"}
  t.Run(name,func(t *testing.T){
   if _,err:=admin.Exec(ctx,"CREATE DATABASE "+pgx.Identifier{name}.Sanitize());err!=nil {t.Fatal(err)}
   defer admin.Exec(ctx,"DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
   u,err:=url.Parse(dsn);if err!=nil {t.Fatal(err)};u.Path="/"+name
   pool,err:=pgxpool.New(ctx,u.String());if err!=nil {t.Fatal(err)};defer pool.Close()
   if upstream {
    goose.SetBaseFS(embedMigrations);if err:=goose.SetDialect("postgres");err!=nil {t.Fatal(err)}
    db:=stdlib.OpenDBFromPool(pool)
    if err:=goose.UpToContext(ctx,db,"migrations",58);err!=nil {t.Fatal(err)}
    db.Close()
   }
   if err:=runMigrations(ctx,pool);err!=nil {t.Fatal(err)}
   if err:=runMigrations(ctx,pool);err!=nil {t.Fatalf("idempotent rerun: %v",err)}
   var n int
   if err:=pool.QueryRow(ctx,`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='public' AND ((table_name='notification_channels' AND column_name IN ('include_profile_name','include_device_name')) OR (table_name='sports_team_channel_links' AND column_name IN ('auto_linked','link_confidence','match_reason')) OR (table_name='sports_teams' AND column_name IN ('location','nickname')) OR (table_name='remote_access_pairings' AND column_name='credential_hash'))`).Scan(&n);err!=nil {t.Fatal(err)}
   if n!=8 {t.Fatalf("expected 8 integrated columns, got %d",n)}
  })
 }
}
