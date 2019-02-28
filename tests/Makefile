include .env.tests
export
export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests bootstrap run integration killapp killdb

tests:
	@$(MAKE) bootstrap
	@$(MAKE) run
	@$(MAKE) integration

integration:
	$(TEST) ./...

killdb:
	@-psql -h $(DB_HOST) -U postgres -c "REVOKE CONNECT ON DATABASE $(DB_NAME) FROM public;" &> /dev/null
	@-psql -h $(DB_HOST) -U postgres -c "SELECT pg_terminate_backend(pg_stat_activity.pid) FROM pg_stat_activity WHERE pg_stat_activity.datname = '$(DB_NAME)';" &> /dev/null
	@-psql -h $(DB_HOST) -U postgres -c "REASSIGN OWNED BY $(DB_USER) TO postgres;" &> /dev/null
	@-psql -h $(DB_HOST) -U postgres $(DB_NAME) -c "DROP OWNED BY $(DB_USER) CASCADE;" &> /dev/null
	@-psql -h $(DB_HOST) -U postgres -c "DROP DATABASE $(DB_NAME);" &> /dev/null
	@-psql -h $(DB_HOST) -U postgres -c "DROP ROLE IF EXISTS $(DB_USER);" &> /dev/null
	@([ $$? -eq 0 ]) || true

killapp:
	@-pkill -f "../bin/app -host $(HOSTNAME)" &> /dev/null
	@([ $$? -eq 0 ]) || true

run: killapp
	@../bin/app -host $(HOSTNAME) -port $(subst :,,$(LISTEN)) &> /dev/null &

bootstrap: killdb
	@cp -r ../db ./
	@../bin/bootstrap -user postgres -seed &> /dev/null
	@rm -rf ./db
