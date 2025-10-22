SHELL := /bin/bash

apodwall: apodwall.go
	go build -o apodwall apodwall.go

.PHONY: clean
clean:
	rm -f apodwall
