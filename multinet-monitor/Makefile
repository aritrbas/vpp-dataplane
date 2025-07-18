include ../common.mk

TAG ?= latest # Tag images with :$(TAG)
VERSION_FILE=version
GENERATE_LOG_FILE=../vpplink/generated/generate.log
ALSO_LATEST ?= n # If 'y' also tag images with :latest

.PHONY: build
build:
	${DOCKER_RUN} go build -o ./watcher .

.PHONY: image
image: build
	@echo "Image tag                   : $(TAG)"                         > $(VERSION_FILE)
	@echo "VPP-dataplane version       : $(shell git log -1 --oneline)" >> $(VERSION_FILE)
	@cat $(GENERATE_LOG_FILE)                                           >> $(VERSION_FILE)
	docker build --pull -t calicovpp/multinet-monitor:$(TAG) .
	@if [ "${ALSO_LATEST}" = "y" ]; then \
		docker tag calicovpp/multinet-monitor:$(TAG) calicovpp/multinet-monitor:latest; \
		docker tag calicovpp/multinet-monitor:$(TAG) calicovpp/multinet-monitor:prerelease; \
	fi

.PHONY: dev
dev: image

push: ${PUSH_DEP}
	set -e; for registry in ${REGISTRIES}; do \
		docker tag calicovpp/multinet-monitor:$(TAG) $${registry}calicovpp/multinet-monitor:$(TAG); \
		if [ "${ALSO_LATEST}" = "y" ]; then \
			docker tag calicovpp/multinet-monitor:latest $${registry}calicovpp/multinet-monitor:latest; \
			docker tag calicovpp/multinet-monitor:prerelease $${registry}calicovpp/multinet-monitor:prerelease; \
		fi; \
		docker push --all-tags $${registry}calicovpp/multinet-monitor; \
	done
