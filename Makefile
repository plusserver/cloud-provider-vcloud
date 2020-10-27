# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
SOURCES := $(shell find . -name '*.go')
GOOS ?= $(shell go env GOOS)
TEST_PATTERN?=.
TEST_OPTIONS?=-v
VERSION ?= $(shell git describe --exact-match 2> /dev/null || \
                 git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
LDFLAGS   := "-w -s -X 'main.version=${VERSION}'"

export PATH := ./bin:$(PATH)
export GO111MODULE=on


cloud-provider-vcloud: $(SOURCES)
	 CGO_ENABLED=0 GOOS=$(GOOS) go build \
		-ldflags $(LDFLAGS) \
		-o bin/vcloud-cloud-controller-manager \
		cmd/vcloud-cloud-controller-manager/main.go

setup:
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh
	go mod download
.PHONY: setup

lint:
	./bin/golangci-lint run --enable-all ./...
.PHONY: lint

# Run all the tests
test:
	go test $(TEST_OPTIONS) -failfast -race -coverpkg=./... -covermode=atomic -coverprofile=coverage.txt $(SOURCES) -run $(TEST_PATTERN) -timeout=2m
.PHONY: test

# Run all the tests and opens the coverage report
cover: test
	go tool cover -html=coverage.txt
.PHONY: cover

clean:
	rm -f vcloud-cloud-controller-manager
.PHONY: clean
