include .env.tests
export
export BUILD_TIME=$(date %s)
export CGO_ENABLED=0
GO := go
TEST := $(GO) test -v

.PHONY: tests
tests: bootstrap run integration

integration: integration_test.go
	$(TEST) integration_test.go
	@pkill -f "../bin/app -host $(HOSTNAME)"

.PHONY: run
run:
	@../bin/app -host $(HOSTNAME) -port $(subst :,,$(LISTEN)) &

bootstrap:
	@psql -h $(DB_HOST) -U postgres  -c "DROP DATABASE IF EXISTS $(DB_NAME)"
	@psql -h $(DB_HOST) -U postgres  -c "DROP ROLE IF EXISTS $(DB_USER)"
	@cp -r ../db ./
	../bin/bootstrap -user postgres -seed
	@rm -rf ./db
