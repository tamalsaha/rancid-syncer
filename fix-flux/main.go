package main

import (
	"fmt"
	"sigs.k8s.io/yaml"
)

func main() {
	y1 := `host: kubedb.dev

paths:
  /elasticsearch-restic-plugin:
    repo: https://github.com/kubedb/elasticsearch-restic-plugin
  /kubedb-manifest-plugin:
    repo: https://github.com/kubedb/kubedb-manifest-plugin
  /mariadb-archiver:
    repo: https://github.com/kubedb/mariadb-archiver
  /mongodb-csi-snapshotter-plugin:
    repo: https://github.com/kubedb/mongodb-csi-snapshotter-plugin
  /mongodb-restic-plugin:
    repo: https://github.com/kubedb/mongodb-restic-plugin
  /mysql-archiver:
    repo: https://github.com/kubedb/mysql-archiver
  /mysql-restic-plugin:
    repo: https://github.com/kubedb/mysql-restic-plugin
  /postgres-archiver:
    repo: https://github.com/kubedb/postgres-archiver
  /redis-restic-plugin:
    repo: https://github.com/kubedb/redis-restic-plugin
  /apimachinery:
    repo: https://github.com/kubedb/apimachinery
  /archiver:
    repo: https://github.com/kubedb/archiver
  /autoscaler:
    repo: https://github.com/kubedb/autoscaler
  /cli:
    repo: https://github.com/kubedb/cli
  /dashboard:
    repo: https://github.com/kubedb/dashboard
  /db-client-go:
    repo: https://github.com/kubedb/db-client-go
  /docs:
    repo: https://github.com/kubedb/docs
  /elasticsearch:
    repo: https://github.com/kubedb/elasticsearch
  /enterprise:
    repo: https://github.com/kubedb/enterprise
  /etcd:
    repo: https://github.com/kubedb/etcd
  /installer:
    repo: https://github.com/kubedb/installer
  /kafka:
    repo: https://github.com/kubedb/kafka
  /mariadb:
    repo: https://github.com/kubedb/mariadb
  /mariadb-coordinator:
    repo: https://github.com/kubedb/mariadb-coordinator
  /memcached:
    repo: https://github.com/kubedb/memcached
  /mongodb:
    repo: https://github.com/kubedb/mongodb
  /mysql:
    repo: https://github.com/kubedb/mysql
  /mysql-coordinator:
    repo: https://github.com/kubedb/mysql-coordinator
  /mysql-replication-mode-detector:
    repo: https://github.com/appscode/mysql-replication-mode-detector
  /mysql-router-init:
    repo: https://github.com/kubedb/mysql-router-init
  /operator:
    repo: https://github.com/kubedb/operator
  /ops-manager:
    repo: https://github.com/kubedb/ops-manager
  /percona-xtradb:
    repo: https://github.com/kubedb/percona-xtradb
  /pg-coordinator:
    repo: https://github.com/kubedb/pg-coordinator
  /pg-leader-election:
    repo: https://github.com/kubedb/pg-leader-election
  /pgbouncer:
    repo: https://github.com/kubedb/pgbouncer
  /pgbouncer_exporter:
    repo: https://github.com/kubedb/pgbouncer_exporter
  /postgres:
    repo: https://github.com/kubedb/postgres
  /profile-charts:
    repo: https://github.com/kubedb/profile-charts
  /profile-kustomizer:
    repo: https://github.com/kubedb/profile-kustomizer
  /provider-aws:
    repo: https://github.com/kubedb/provider-aws
  /provider-azure:
    repo: https://github.com/kubedb/provider-azure
  /provider-gcp:
    repo: https://github.com/kubedb/provider-gcp
  /provisioner:
    repo: https://github.com/kubedb/provisioner
  /proxysql:
    repo: https://github.com/kubedb/proxysql
  /redis:
    repo: https://github.com/kubedb/redis
  /redis-coordinator:
    repo: https://github.com/kubedb/redis-coordinator
  /redis-node-finder:
    repo: https://github.com/kubedb/redis-node-finder
  /replication-mode-detector:
    repo: https://github.com/kubedb/replication-mode-detector
  /schema-manager:
    repo: https://github.com/kubedb/schema-manager
  /singlestore:
    repo: https://github.com/kubedb/singlestore
  /singlestore-coordinator:
    repo: https://github.com/kubedb/singlestore-coordinator
  /tests:
    repo: https://github.com/kubedb/tests
  /timescale:
    repo: https://github.com/kubedb/timescale
  /ui-server:
    repo: https://github.com/kubedb/ui-server
  /webhook-server:
    repo: https://github.com/kubedb/webhook-server

`

	var obj map[string]any
	err := yaml.Unmarshal([]byte(y1), &obj)
	if err != nil {
		panic(err)
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
