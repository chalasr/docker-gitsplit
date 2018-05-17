# A Docker image with git and splitsh-lite

See [the official site](https://github.com/splitsh/lite) for more information about splitsh.

# Usage

Include a `.gitsplit.yml` file in the root of your repository.
This section provides a brief overview of the configuration file and split process.

Use env variable to inject your credential and manage authentication.

Example `.gitsplit.yml` configuration:

```yaml
# Used to speed up the split over time by reusing git's objects
cache_dir: "/cache/gitsplit"

# Path to the repository to split (default = current path)
project_dir: /home/me/workspace/another_project

# List of splits.
splits:
  - prefix: "src/partA"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partA.git"
  - prefix: "src/partB"
    target:
      # You can push the split to several repositories
      - "https://${GH_TOKEN}@github.com/my_company/project-partB.git"
      - "https://${GH_TOKEN}@github.com/my_company/project-partZ.git"
  - prefix:
      # You can use several prefix in the split
      - "src/subTree/PartC"
      - "src/subTree/PartZ"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partC.git"

# List of references to split (defined as regexp)
origins:
  - ^master$
  - ^develop$
  - ^feature/
  - ^v\d+\.\d+\.\d+$
```

# Split your repo manualy

With a github token:
```
$ docker run --rm -ti -e GH_TOKEN -v /cache:/cache/gitsplit -v $PWD:/srv jderusse/gitsplit
```

With ssh agent:
```
$ docker run --rm -ti -e SSH_AUTH_SOCK=/ssh-agent -v $SSH_AUTH_SOCK:/ssh-agent -v /cache:/cache/gitsplit -v $PWD:/srv jderusse/gitsplit
```

# Sample with drone.io

Beware, the container have to push on your splited repository.
It could be a security issue. Use environments variables as defined in the official documentation

```yaml
# .gitsplit.yml
cache_dir: "/cache/gitsplit"
splits:
  - prefix: "src/partA"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partA.git"
  - prefix: "src/partB"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partB.git"
  - prefix: "src/subTree/PartC"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partC.git"

origins:
  - ^master$
  - ^develop$
```

```yaml
# .drone.yml
pipeline:
  split:
    image: jderusse/gitsplit
    pull: true
    volumes:
      # Share a cache mounted in the runner
      - /drone/cache/gitsplit:/cache/gitsplit

      # Use ssh key defined in the runner
      - /drone/env/gitsplit.ssh:/root/.ssh/
    commands:
      # have to fetch remote branches
      - git fetch --prune --unshallow || true
      - gitsplit
```

# Sample with Travis CI

Beware, the container have to push on your splited repository.
It could be a security issue. Use environments variables as defined in the official documentation

```yaml
# .gitsplit.yml
cache_dir: "/cache/gitsplit"
splits:
  - prefix: "src/partA"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partA.git"
  - prefix: "src/partB"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partB.git"
  - prefix: "src/subTree/PartC"
    target: "https://${GH_TOKEN}@github.com/my_company/project-partC.git"

origins:
  - ^master$
  - ^develop$
```

```yaml
# .travis.yml
sudo: required
services:
  - docker
cache:
  directories:
    - /cache/gitsplit
install:
  - docker pull jderusse/gitsplit

  # update local repository. Because travis fetch a shallow copy
  - git config remote.origin.fetch "+refs/*:refs/*"
  - git config remote.origin.mirror true
  - git fetch --prune --unshallow || true

script:
  - docker run --rm -t -e GH_TOKEN -v /cache/gitsplit:/cache/gitsplit -v ${PWD}:/srv jderusse/gitsplit --ref "${TRAVIS_BRANCH}"
```

# Sample with GitLab CI/CD

Beware, the container have to push on your splited repository.
It could be a security issue. Use environments variables as defined in the official documentation [GitLab SSH Deploy keys](https://docs.gitlab.com/ce/ssh/README.html#deploy-keys).

Note: I highly recommend to use ssh instead of https because of the username/password or username/token. Deploy keys are much easier to use with GitLab

```yaml
# .gitsplit.yml
cache_dir: "cache/gitsplit"
splits:
  - prefix: "src/partA"
    target: "git@gitlab.com:my_company/project-partA.git"
  - prefix: "src/partB"
    target: "git@gitlab.com:my_company/project-partB.git"
  - prefix: "src/subTree/PartC"
    target: "git@gitlab.com:my_company/project-partC.git"

origins:
  - ^master$
  - ^develop$
```

```yaml
# .gitlab-ci.yml with Docker runners
stages:
  - split

split:
  image: jderusse/gitsplit
  stage: split
  cache:
    key: "$CI_JOB_NAME-$CI_COMMIT_REF_NAME"
    paths:
      - cache/gitsplit
  variables:
    GIT_STRATEGY: clone
    GIT_CHECKOUT: "false"
    GIT_DEPTH: 1
  before_script:
    - eval $(ssh-agent -s)
    - mkdir -p ~/.ssh
    - chmod 700 ~/.ssh
    - echo -e "Host *\n\tStrictHostKeyChecking no\n\n" > ~/.ssh/config
    - echo "$SSH_PRIVATE_KEY" | tr -d '\r' | ssh-add - > /dev/null
    - ssh-add -l
  script:
    - git config remote.origin.fetch "+refs/*:refs/*"
    - git config remote.origin.mirror true
    - git fetch --prune
    - gitsplit --ref "${CI_COMMIT_REF_NAME}"
```
