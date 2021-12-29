// Copyright 2021 The jackal Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package xep0198

import (
	"context"

	c2smodel "github.com/ortuman/jackal/pkg/model/c2s"
	"github.com/ortuman/jackal/pkg/router"
	"github.com/ortuman/jackal/pkg/router/stream"
)

//go:generate moq -out router.mock_test.go . globalRouter:routerMock
type globalRouter interface {
	router.Router
}

//go:generate moq -out c2s_stream.mock_test.go . c2sStream
type c2sStream interface {
	stream.C2S
}

//go:generate moq -out resourcemanager.mock_test.go . resourceManager
type resourceManager interface {
	GetResource(ctx context.Context, username, resource string) (c2smodel.ResourceDesc, error)
}
