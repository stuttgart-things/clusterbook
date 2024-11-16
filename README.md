# stuttgart-things/clusterbook

gitops configuration service

<div align="center">
  <p>
    <img src="https://github.com/stuttgart-things/docs/blob/main/hugo/sthings-argo.png" alt="sthings" width="450" />
  </p>
  <p>
    <strong>[/ˈklʌstəʳbʊk/]</strong>- gitops cluster configuration management

  </p>
</div>

## DEV TASKS

```bash
task: Available tasks for this project:
* branch:         Create branch from main
* build:          Install
* build-ko:       Build image w/ KO
* commit:         Commit + push code into branch
* lint:           Lint Golang
* pr:             Create pull request into main
* proto:          Generate Go code from proto
* run:            Run
* test:           Test code
```

## AUTHOR

```bash
Patrick Hermann, stuttgart-things 09/2024
```

## EXAMPLE .env file

<details><summary>ENV FILE</summary>

.env file needed for Taskfile

```bash
cat <<EOF > .env
#LOAD_CONFIG_FROM=disk
#CONFIG_LOCATION=tests
#CONFIG_NAME=config.yaml
LOAD_CONFIG_FROM=cr
CONFIG_LOCATION=clusterbook #namespace
CONFIG_NAME=networks-labul #resource-name

SERVER_PORT=50051

#CLUSTERBOOK_SERVER=localhost:50051
#SECURE_CONNECTION=false
CLUSTERBOOK_SERVER=clusterbook.rke2.sthings-vsphere.labul.sva.de:443
SECURE_CONNECTION=true
EOF
```

</details>

## LICENSE

Licensed under the Apache License, Version 2.0 (the "License").

You may obtain a copy of the License at [apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an _"AS IS"_ basis, without WARRANTIES or conditions of any kind, either express or implied.

See the License for the specific language governing permissions and limitations under the License.