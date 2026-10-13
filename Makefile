.PHONY: format format-vue format-go

format: format-vue format-go

format-vue:
	npm --prefix apps/frontend run format

format-go:
	find apps/backend -type f -name '*.go' -exec gofmt -w {} +
