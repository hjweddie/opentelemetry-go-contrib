// Copyright The OpenTelemetry Authors
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

// Based on https://github.com/DataDog/dd-trace-go/blob/8fb554ff7cf694267f9077ae35e27ce4689ed8b6/contrib/gin-gonic/gin/gintrace_test.go

package otelgin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	b3prop "go.opentelemetry.io/contrib/propagators/b3"
)

func init() {
	gin.SetMode(gin.ReleaseMode) // silence annoying log msgs
}

func TestGetSpanNotInstrumented(t *testing.T) {
	router := gin.New()
	router.GET("/ping", func(c *gin.Context) {
		// Assert we don't have a span on the context.
		span := trace.SpanFromContext(c.Request.Context())
		ok := !span.SpanContext().IsValid()
		assert.True(t, ok)
		_, _ = c.Writer.Write([]byte("ok"))
	})
	r := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	response := w.Result()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestPropagationWithGlobalPropagators(t *testing.T) {
	provider := trace.NewNoopTracerProvider()
	otel.SetTextMapPropagator(b3prop.New())

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx, _ = provider.Tracer(tracerName).Start(ctx, "test")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(r.Header))

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider)))
	router.GET("/user/:id", func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		assert.Equal(t, sc.TraceID(), span.SpanContext().TraceID())
		assert.Equal(t, sc.SpanID(), span.SpanContext().SpanID())
	})

	router.ServeHTTP(w, r)
}

func TestPropagationWithCustomPropagators(t *testing.T) {
	provider := trace.NewNoopTracerProvider()
	b3 := b3prop.New()

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	ctx := context.Background()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01},
		SpanID:  trace.SpanID{0x01},
	})
	ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	ctx, _ = provider.Tracer(tracerName).Start(ctx, "test")
	b3.Inject(ctx, propagation.HeaderCarrier(r.Header))

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider), WithPropagators(b3)))
	router.GET("/user/:id", func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		assert.Equal(t, sc.TraceID(), span.SpanContext().TraceID())
		assert.Equal(t, sc.SpanID(), span.SpanContext().SpanID())
	})

	router.ServeHTTP(w, r)
}

func TestWithCustomSpanStartOptions(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	// test for override span start time, it may be useless
	spanStartTime := time.Now().Add(time.Second)

	startOpts := []trace.SpanStartOption{
		trace.WithTimestamp(spanStartTime),
		trace.WithAttributes(
			[]attribute.KeyValue{
				attribute.String("foo", "bar"),
			}...,
		),
	}

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider), WithSpanStartOptions(startOpts...)))

	router.GET("/user/:id", func(c *gin.Context) {
		// it does not need anything
	})

	router.ServeHTTP(w, r)

	spans := spanRecorder.Ended()

	// check for span start time
	assert.Equal(t, spans[0].StartTime(), spanStartTime, "Span start time should be equal to overrided one")

	// check if "foo":"bar" pairs is in span's attributes
	attr := attribute.KeyValue{}
	for _, attr = range spans[0].Attributes() {
		if attr.Key == "foo" {
			break
		}
	}

	assert.Equal(t, attr.Value.AsString(), "bar", "It should have foo bar attribute in the first span")
}

func TestWithCustomSpanEndOptions(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	r := httptest.NewRequest("GET", "/user/123", nil)
	w := httptest.NewRecorder()

	// test for override span end time, it may be useless
	spanEndTime := time.Now().Add(time.Minute)

	endOpts := []trace.SpanEndOption{
		trace.WithTimestamp(spanEndTime),
		trace.WithStackTrace(true), // capture the error with stack trace
	}

	router := gin.New()
	router.Use(Middleware("foobar", WithTracerProvider(provider), WithSpanEndOptions(endOpts...)))

	router.GET("/user/:id", func(c *gin.Context) {
		// it does not need anything
	})

	router.ServeHTTP(w, r)

	spans := spanRecorder.Ended()

	// check for span end time
	assert.Equal(t, spans[0].EndTime(), spanEndTime, "Span end time should be equal to overrided one")
}
