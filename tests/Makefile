include .env.tests
export
export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests bootstrap run integration

tests: bootstrap run integration

integration:
	$(TEST) integration_test.go
	@pkill -f "../bin/app -host $(HOSTNAME)"

run:
	@../bin/app -host $(HOSTNAME) -port $(subst :,,$(LISTEN)) &

bootstrap:
	@psql -h $(DB_HOST) -U postgres  -c "DROP DATABASE IF EXISTS $(DB_NAME)"
	@psql -h $(DB_HOST) -U postgres  -c "DROP ROLE IF EXISTS $(DB_USER)"
	@cp -r ../db ./
	../bin/bootstrap -user postgres -seed
	@rm -rf ./db
