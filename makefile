DOCKER_IMAGE     := jieht9u/go-redis-raft
DOCKERFILE_LOCAL := dockerfile

GITVERSION		 := 0.0.1

DATE             := $(shell date -u '+%Y-%m-%d-%H:%M UTC')
LDFLAGS    		 := '-X "main.Version=$(GITVERSION)" -X "main.BuildTime=$(DATE)"'

DIST_DIR 		 := $(CURDIR)/dist


.PHONY: docker
docker: docker-build docker-push
	@echo "Success Docker"

.PHONY: docker-build
docker-build:
	@echo "Docker Build..."
	$Q docker build -t $(DOCKER_IMAGE):$(GITVERSION) --build-arg LDFLAGS=$(LDFLAGS) --file=$(DOCKERFILE_LOCAL) .

.PHONY: docker-push
docker-push:
	@echo "Docker Push..."
	$Q docker tag $(DOCKER_IMAGE):$(GITVERSION) $(DOCKER_IMAGE):$(GITVERSION) 
	$Q docker push $(DOCKER_IMAGE):$(GITVERSION)


#TAG RELEASE
.PHONY: tag-release
tag-release: tag 
	$Q git push origin v$(GITVERSION)

# RELEASE
.PHONY: release
release: clean-dist tag 
	$Q goreleaser	

.PHONY: clean-dist
clean-dist:
	@echo "Removing distribution files"
	rm -rf $(DIST_DIR)

.PHONY: tags echo
tags:
	@echo "Listing tags..."
	$Q @git tag

echo:
	@echo "MESSAGE " $(MESSAGE)


.PHONY: tag
tag:
	@echo "Creating tag" $(GITVERSION)
	$Q @git tag -a v$(GITVERSION) -m $(GITVERSION)

.PHONY: td
td:
	@echo "Delete tag" $(GITVERSION)
	$Q @git tag -d v$(GITVERSION) 

#MERGE
.PHONY: merge
merge:
	$Q git checkout master
	$Q git merge dev	