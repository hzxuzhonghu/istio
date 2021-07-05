// Copyright Istio Authors
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

package route

import (
	"reflect"
	"testing"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	xdstype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/gogo/protobuf/types"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config/labels"
)

func TestIsCatchAllRoute(t *testing.T) {
	cases := []struct {
		name  string
		route *route.Route
		want  bool
	}{
		{
			name: "catch all prefix",
			route: &route.Route{
				Name: "catch-all",
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: "/",
					},
				},
			},
			want: true,
		},
		{
			name: "catch all regex",
			route: &route.Route{
				Name: "catch-all",
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_SafeRegex{
						SafeRegex: &matcher.RegexMatcher{
							EngineType: &matcher.RegexMatcher_GoogleRe2{GoogleRe2: &matcher.RegexMatcher_GoogleRE2{}},
							Regex:      "*",
						},
					},
				},
			},
			want: true,
		},
		{
			name: "uri regex with headers",
			route: &route.Route{
				Name: "non-catch-all",
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_SafeRegex{
						SafeRegex: &matcher.RegexMatcher{
							// nolint: staticcheck
							EngineType: &matcher.RegexMatcher_GoogleRe2{},
							Regex:      "*",
						},
					},
					Headers: []*route.HeaderMatcher{
						{
							Name: "Authentication",
							HeaderMatchSpecifier: &route.HeaderMatcher_SafeRegexMatch{
								SafeRegexMatch: &matcher.RegexMatcher{
									// nolint: staticcheck
									EngineType: &matcher.RegexMatcher_GoogleRe2{},
									Regex:      "*",
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "uri regex with query params",
			route: &route.Route{
				Name: "non-catch-all",
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_SafeRegex{
						SafeRegex: &matcher.RegexMatcher{
							// nolint: staticcheck
							EngineType: &matcher.RegexMatcher_GoogleRe2{},
							Regex:      "*",
						},
					},
					QueryParameters: []*route.QueryParameterMatcher{
						{
							Name: "Authentication",
							QueryParameterMatchSpecifier: &route.QueryParameterMatcher_PresentMatch{
								PresentMatch: true,
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			catchall := isCatchAllRoute(tt.route)
			if catchall != tt.want {
				t.Errorf("Unexpected catchAllMatch want %v, got %v", tt.want, catchall)
			}
		})
	}
}

func TestTranslateCORSPolicy(t *testing.T) {
	corsPolicy := &networking.CorsPolicy{
		AllowOrigins: []*networking.StringMatch{
			{MatchType: &networking.StringMatch_Exact{Exact: "exact"}},
			{MatchType: &networking.StringMatch_Prefix{Prefix: "prefix"}},
			{MatchType: &networking.StringMatch_Regex{Regex: "regex"}},
		},
	}
	expectedCorsPolicy := &route.CorsPolicy{
		AllowOriginStringMatch: []*matcher.StringMatcher{
			{MatchPattern: &matcher.StringMatcher_Exact{Exact: "exact"}},
			{MatchPattern: &matcher.StringMatcher_Prefix{Prefix: "prefix"}},
			{
				MatchPattern: &matcher.StringMatcher_SafeRegex{
					SafeRegex: &matcher.RegexMatcher{
						EngineType: regexEngine,
						Regex:      "regex",
					},
				},
			},
		},
		EnabledSpecifier: &route.CorsPolicy_FilterEnabled{
			FilterEnabled: &core.RuntimeFractionalPercent{
				DefaultValue: &xdstype.FractionalPercent{
					Numerator:   100,
					Denominator: xdstype.FractionalPercent_HUNDRED,
				},
			},
		},
	}
	if got := translateCORSPolicy(corsPolicy); !reflect.DeepEqual(got, expectedCorsPolicy) {
		t.Errorf("translateCORSPolicy() = \n%v, want \n%v", got, expectedCorsPolicy)
	}
}

func TestMirrorPercent(t *testing.T) {
	cases := []struct {
		name  string
		route *networking.HTTPRoute
		want  *core.RuntimeFractionalPercent
	}{
		{
			name: "zero mirror percent",
			route: &networking.HTTPRoute{
				Mirror:        &networking.Destination{},
				MirrorPercent: &types.UInt32Value{Value: 0.0},
			},
			want: nil,
		},
		{
			name: "mirror with no value given",
			route: &networking.HTTPRoute{
				Mirror: &networking.Destination{},
			},
			want: &core.RuntimeFractionalPercent{
				DefaultValue: &xdstype.FractionalPercent{
					Numerator:   100,
					Denominator: xdstype.FractionalPercent_HUNDRED,
				},
			},
		},
		{
			name: "mirror with actual percent",
			route: &networking.HTTPRoute{
				Mirror:        &networking.Destination{},
				MirrorPercent: &types.UInt32Value{Value: 50},
			},
			want: &core.RuntimeFractionalPercent{
				DefaultValue: &xdstype.FractionalPercent{
					Numerator:   50,
					Denominator: xdstype.FractionalPercent_HUNDRED,
				},
			},
		},
		{
			name: "zero mirror percentage",
			route: &networking.HTTPRoute{
				Mirror:           &networking.Destination{},
				MirrorPercentage: &networking.Percent{Value: 0.0},
			},
			want: nil,
		},
		{
			name: "mirrorpercentage with actual percent",
			route: &networking.HTTPRoute{
				Mirror:           &networking.Destination{},
				MirrorPercentage: &networking.Percent{Value: 50.0},
			},
			want: &core.RuntimeFractionalPercent{
				DefaultValue: &xdstype.FractionalPercent{
					Numerator:   500000,
					Denominator: xdstype.FractionalPercent_MILLION,
				},
			},
		},
		{
			name: "mirrorpercentage takes precedence when both are given",
			route: &networking.HTTPRoute{
				Mirror:           &networking.Destination{},
				MirrorPercent:    &types.UInt32Value{Value: 40},
				MirrorPercentage: &networking.Percent{Value: 50.0},
			},
			want: &core.RuntimeFractionalPercent{
				DefaultValue: &xdstype.FractionalPercent{
					Numerator:   500000,
					Denominator: xdstype.FractionalPercent_MILLION,
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			mp := mirrorPercent(tt.route)
			if !reflect.DeepEqual(mp, tt.want) {
				t.Errorf("Unexpected mirro percent want %v, got %v", tt.want, mp)
			}
		})
	}
}

func TestSourceMatchHTTP(t *testing.T) {
	type args struct {
		match          *networking.HTTPMatchRequest
		proxyLabels    labels.Collection
		gatewayNames   map[string]bool
		proxyNamespace string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"source namespace match",
			args{
				match: &networking.HTTPMatchRequest{
					SourceNamespace: "foo",
				},
				proxyNamespace: "foo",
			},
			true,
		},
		{
			"source namespace not match",
			args{
				match: &networking.HTTPMatchRequest{
					SourceNamespace: "foo",
				},
				proxyNamespace: "bar",
			},
			false,
		},
		{
			"source namespace not match when empty",
			args{
				match: &networking.HTTPMatchRequest{
					SourceNamespace: "foo",
				},
				proxyNamespace: "",
			},
			false,
		},
		{
			"source namespace any",
			args{
				match:          &networking.HTTPMatchRequest{},
				proxyNamespace: "bar",
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceMatchHTTP(tt.args.match, tt.args.proxyLabels, tt.args.gatewayNames, tt.args.proxyNamespace); got != tt.want {
				t.Errorf("sourceMatchHTTP() = %v, want %v", got, tt.want)
			}
		})
	}
}
