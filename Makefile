.PHONY: test test-service-a test-service-b

# Run the tests of both services.
test: test-service-a test-service-b

test-service-a:
	cd service-a && go test -count=1 ./...

test-service-b:
	cd service-b && go test -count=1 ./...
