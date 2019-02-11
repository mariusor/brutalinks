include .env.tests
export
export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests bootstrap run integration killapp

tests: bootstrap run integration

integration:
	$(TEST) ./...
	$(MAKE) killapp

killapp:
	@-pkill -f "../bin/app -host $(HOSTNAME)"
	@([ $$? -eq 0 ]) || true

run: killapp
	@../bin/app -host $(HOSTNAME) -port $(subst :,,$(LISTEN)) &

bootstrap:
	@psql -h $(DB_HOST) -U postgres  -c "DROP DATABASE IF EXISTS $(DB_NAME)"
	@psql -h $(DB_HOST) -U postgres  -c "DROP ROLE IF EXISTS $(DB_USER)"
	@cp -r ../db ./
	../bin/bootstrap -user postgres -seed
	@rm -rf ./db
