/* Copyright (c) 2019 Snowflake Inc. All rights reserved.

   Licensed under the Apache License, Version 2.0 (the
   "License"); you may not use this file except in compliance
   with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing,
   software distributed under the License is distributed on an
   "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
   KIND, either express or implied.  See the License for the
   specific language governing permissions and limitations
   under the License.
*/

// Package rpcauth provides OPA policy authorization
// for Sansshell RPCs.
package rpcauth

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/Snowflake-Labs/sansshell/auth/opa"
)

// An Authorizer performs authorization of Sanshsell RPCs based on
// an OPA/Rego policy.
//
// It can be used as both a unary and stream interceptor, or manually
// invoked to perform policy checks using `Eval`
type Authorizer struct {
	// The AuthzPolicy used to perform authorization checks.
	policy *opa.AuthzPolicy

	// Additional authorization hooks invoked before policy evaluation.
	hooks []RPCAuthzHook
}

// A RPCAuthzHook is invoked on populated RpcAuthInput prior to policy
// evaluation, and may augment / mutate the input, or pre-emptively
// reject a request.
type RPCAuthzHook interface {
	Hook(context.Context, *RPCAuthInput) error
}

// New creates a new Authorizer from an opa.AuthzPolicy. Any supplied authorization
// hooks will be executed, in the order provided, on each policy evauluation.
// NOTE: The policy is used for both client and server hooks below. If you need
//       distinct policy for client vs server, create 2 Authorizer's.
func New(policy *opa.AuthzPolicy, authzHooks ...RPCAuthzHook) *Authorizer {
	return &Authorizer{policy: policy, hooks: authzHooks}
}

// NewWithPolicy creates a new Authorizer from a policy string. Any supplied
// authorization hooks will be executed, in the order provided, on each policy
// evaluation.
// NOTE: The policy is used for both client and server hooks below. If you need
//       distinct policy for client vs server, create 2 Authorizer's.
func NewWithPolicy(ctx context.Context, policy string, authzHooks ...RPCAuthzHook) (*Authorizer, error) {
	p, err := opa.NewAuthzPolicy(ctx, policy)
	if err != nil {
		return nil, err
	}
	return New(p, authzHooks...), nil
}

// Eval will evalulate the supplied input against the authorization policy, returning
// nil iff policy evaulation was successful, and the request is permitted, or
// an appropriate status.Error otherwise. Any input hooks will be executed
// prior to policy evaluation, and may mutate `input`, regardless of the
// the success or failure of policy.
func (g *Authorizer) Eval(ctx context.Context, input *RPCAuthInput) error {
	logger := logr.FromContextOrDiscard(ctx)
	if input != nil {
		if logger.V(2).Enabled() {
			b, err := json.Marshal(input)
			if err != nil {
				logger.V(2).Info("marshal", "can't marshal input", err)
			} else {
				logger.V(2).Info("evaluating authz policy", "input", string(b))
			}
		}
	}
	if input == nil {
		return status.Error(codes.InvalidArgument, "policy input cannot be nil")
	}
	for _, hook := range g.hooks {
		if err := hook.Hook(ctx, input); err != nil {
			if _, ok := status.FromError(err); ok {
				// error is already an appropriate status.Status
				return err
			}
			return status.Errorf(codes.Internal, "authz hook error: %v", err)
		}
	}
	if logger.V(1).Enabled() {
		b, err := json.Marshal(input)
		if err != nil {
			logger.V(1).Info("marshal", "can't marshal input", err)
		} else {
			logger.V(1).Info("evaluating authz policy post hooks", "input", string(b))
		}
	}
	allowed, err := g.policy.Eval(ctx, input)
	if err != nil {
		return status.Errorf(codes.Internal, "authz policy evaluation error: %v", err)
	}
	if !allowed {
		logger.V(1).Info("permission denied")
		return status.Errorf(codes.PermissionDenied, "OPA policy does not permit this request")
	}
	return nil
}

// Authorize implements grpc.UnaryServerInterceptor
func (g *Authorizer) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	msg, ok := req.(proto.Message)
	if !ok {
		return nil, status.Errorf(codes.Internal, "unable to authorize request of type %T which is not proto.Message", req)
	}
	authInput, err := NewRPCAuthInput(ctx, info.FullMethod, msg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unable to create auth input: %v", err)
	}
	if err := g.Eval(ctx, authInput); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// AuthorizeClient implements grpc.UnaryClientInterceptor
func (g *Authorizer) AuthorizeClient(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	msg, ok := req.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "unable to authorize request of type %T which is not proto.Message", req)
	}
	authInput, err := NewRPCAuthInput(ctx, method, msg)
	if err != nil {
		return status.Errorf(codes.Internal, "unable to create auth input: %v", err)
	}
	if err := g.Eval(ctx, authInput); err != nil {
		return err
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

// AuthorizeClientStream implements grpc.StreamClientInterceptor and applies policy checks on any SendMsg calls.
func (g *Authorizer) AuthorizeClientStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	clientStream, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't create clientStream: %v", err)
	}
	wrapped := &wrappedClientStream{
		ClientStream: clientStream,
		method:       method,
		authz:        g,
	}
	return wrapped, nil
}

// wrappedClientStream wraps an existing grpc.ClientStream with authorization checking.
type wrappedClientStream struct {
	grpc.ClientStream
	method string
	authz  *Authorizer
}

// see: grpc.ClientStream.SendMsg
func (e *wrappedClientStream) SendMsg(req interface{}) error {
	ctx := e.Context()
	msg, ok := req.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "unable to authorize request of type %T which is not proto.Message", req)
	}
	authInput, err := NewRPCAuthInput(ctx, e.method, msg)
	if err != nil {
		return err
	}
	if err := e.authz.Eval(ctx, authInput); err != nil {
		return err
	}
	return e.ClientStream.SendMsg(req)
}

// AuthorizeStream implements grpc.StreamServerInterceptor and applies policy checks on any RecvMsg calls.
func (g *Authorizer) AuthorizeStream(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	wrapped := &wrappedStream{
		ServerStream: ss,
		info:         info,
		authz:        g,
	}
	return handler(srv, wrapped)
}

// wrappedStream wraps an existing grpc.ServerStream with authorization checking.
type wrappedStream struct {
	grpc.ServerStream
	info  *grpc.StreamServerInfo
	authz *Authorizer
}

// see: grpc.ServerStream.RecvMsg
func (e *wrappedStream) RecvMsg(req interface{}) error {
	ctx := e.Context()
	// The argument to RecvMsg is the destination message, which will
	// be filled by the stream.
	// Therefore, in order to check the message against the policy, it
	// first needs to be populated by receiving from the wire.
	if err := e.ServerStream.RecvMsg(req); err != nil {
		return err
	}
	msg, ok := req.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "unable to authorize request of type %T which is not proto.Message", req)
	}
	authInput, err := NewRPCAuthInput(ctx, e.info.FullMethod, msg)
	if err != nil {
		return err
	}
	if err := e.authz.Eval(ctx, authInput); err != nil {
		return err
	}
	return nil
}
