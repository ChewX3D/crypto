.PHONY: gen-mocks

gen-mocks:
	rm -frd mocks
	mockery --config configs/.mockery.yml

test:
	gotestsum ./...
