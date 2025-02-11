// Copyright 2016 Google LLC. All Rights Reserved.
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

// Package ratelimiter provides an exceedingly simple rate limiter.
package ratelimiter

import (
	"context"

	"golang.org/x/time/rate"
)

// Limiter is a simple rate limiter.
type Limiter struct {
	ctx    context.Context
	bucket *rate.Limiter
}

// Wait blocks for the amount of time required by the Limiter so as to not
// exceed its rate.
func (l *Limiter) Wait() {
	l.bucket.Wait(l.ctx)
}

// NewLimiter creates a new Limiter with a rate of limit per second.
func NewLimiter(limit int) *Limiter {
	return &Limiter{ctx: context.Background(),
		bucket: rate.NewLimiter(rate.Limit(limit), 1)}
}
