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

package horaedb

import (
	"context"
	"strings"

	"github.com/pkg/errors"
)

type clientImpl struct {
	rpcClient   *rpcClient
	routeClient routeClient
}

func newClient(endpoint string, routeMode RouteMode, opts options) (Client, error) {
	rpcClient := newRPCClient(opts)
	routeClient, err := newRouteClient(endpoint, routeMode, rpcClient, opts)
	if err != nil {
		return nil, err
	}
	return &clientImpl{
		rpcClient:   rpcClient,
		routeClient: routeClient,
	}, nil
}

func shouldClearRoute(err error) bool {
	if err != nil {
		if unwrapErr, ok := err.(*Error); ok && unwrapErr.ShouldClearRoute() {
			return true
		} else if strings.Contains(err.Error(), "connection error") {
			// TODO: Find a better way to check if err means remote endpoint is down.
			return true
		}
	}

	return false
}

func (c *clientImpl) SQLQuery(ctx context.Context, req SQLQueryRequest) (SQLQueryResponse, error) {
	if err := c.withDefaultRequestContext(&req.ReqCtx); err != nil {
		return SQLQueryResponse{}, errors.Wrap(err, "add request ctx")
	}

	if len(req.Tables) == 0 {
		return SQLQueryResponse{}, ErrNullRequestTables
	}

	routes, err := c.routeClient.RouteFor(req.ReqCtx, req.Tables)
	if err != nil {
		return SQLQueryResponse{}, errors.Wrapf(err, "route tables failed, names:%v", req.Tables)
	}

	var endpoint string
	if v, ok := routes[req.Tables[0]]; ok {
		endpoint = v.Endpoint
	} else {
		return SQLQueryResponse{}, errors.Wrapf(ErrEmptyRoute, "failed to route table, name:%s", req.Tables[0])
	}

	resp, err := c.rpcClient.SQLQuery(ctx, endpoint, req)
	if err != nil {
		if shouldClearRoute(err) {
			c.routeClient.ClearRouteFor(req.Tables)
		}

		return SQLQueryResponse{}, errors.Wrap(err, "do grpc query")
	}

	return resp, nil
}

func (c *clientImpl) Write(ctx context.Context, req WriteRequest) (WriteResponse, error) {
	if err := c.withDefaultRequestContext(&req.ReqCtx); err != nil {
		return WriteResponse{}, errors.Wrap(err, "add request ctx")
	}

	if len(req.Points) == 0 {
		return WriteResponse{}, ErrNullRows
	}

	tables := getTablesFromPoints(req.Points)
	routes, err := c.routeClient.RouteFor(req.ReqCtx, tables)
	if err != nil {
		return WriteResponse{}, errors.Wrap(err, "route table")
	}

	pointsByRoute, err := splitPointsByRoute(req.Points, routes)
	if err != nil {
		return WriteResponse{}, errors.Wrap(err, "split points by route")
	}

	// TODO(chenxiang): Convert to parallel write
	ret := WriteResponse{}
	for endpoint, points := range pointsByRoute {
		response, err := c.rpcClient.Write(ctx, endpoint, req.ReqCtx, points)
		if err != nil {
			if shouldClearRoute(err) {
				c.routeClient.ClearRouteFor(getTablesFromPoints(points))
			}

			// Only return first error message now.
			if ret.Message == "" {
				ret.Message = err.Error()
			}
			ret = combineWriteResponse(ret, WriteResponse{Failed: uint32(len(points))})
			continue
		}

		ret = combineWriteResponse(ret, response)
	}

	return ret, nil
}

func (c *clientImpl) withDefaultRequestContext(reqCtx *RequestContext) error {
	// use default
	if reqCtx.Database == "" {
		reqCtx.Database = c.rpcClient.opts.Database
	}

	// check Request Context
	if reqCtx.Database == "" {
		return ErrNoDatabaseSelected
	}
	return nil
}
