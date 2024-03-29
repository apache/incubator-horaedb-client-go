/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package test

import (
	"context"
	"os"
	"testing"

	"github.com/apache/horaedb-client-go/horaedb"
	"github.com/stretchr/testify/require"
)

var clusterEndpoint = "127.0.0.1:8831"

func init() {
	if v := os.Getenv("HORAEDB_CLUSTER_ADDR"); v != "" {
		clusterEndpoint = v
	}
}

func TestClusterMultiWriteAndQuery(t *testing.T) {
	t.Skip("ignore local test")

	client, err := horaedb.NewClient(clusterEndpoint, horaedb.Direct,
		horaedb.EnableLoggerDebug(true),
	)
	require.NoError(t, err, "init horaedb client failed")

	timestamp := currentMS()

	table1Points, err := buildTablePoints("horaedb_route_test1", timestamp, 2)
	require.NoError(t, err, "build table1 points failed")

	table2Points, err := buildTablePoints("horaedb_route_test2", timestamp, 3)
	require.NoError(t, err, "build table2 points failed")

	table1Points = append(table1Points, table2Points...)

	req := horaedb.WriteRequest{
		Points: table1Points,
	}
	resp, err := client.Write(context.Background(), req)
	require.NoError(t, err, "write rows failed")

	require.Equal(t, resp.Success, uint32(5), "write success value is not expected")

	testBaseQuery(t, client, "horaedb_route_test1", timestamp, 2)
	testBaseQuery(t, client, "horaedb_route_test2", timestamp, 3)
	t.Log("multi table write is paas")
}
