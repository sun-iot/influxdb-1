package authorization

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/influxdata/influxdb/v2"
	"github.com/influxdata/influxdb/v2/inmem"
	"github.com/influxdata/influxdb/v2/kv"
	"github.com/influxdata/influxdb/v2/kv/migration/all"
	"github.com/influxdata/influxdb/v2/pkg/pointer"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestAuth(t *testing.T) {
	setup := func(t *testing.T, store *Store, tx kv.Tx) {
		for i := 1; i <= 10; i++ {
			err := store.CreateAuthorization(context.Background(), tx, &influxdb.Authorization{
				ID:     influxdb.ID(i),
				Token:  fmt.Sprintf("randomtoken%d", i),
				OrgID:  influxdb.ID(i),
				UserID: influxdb.ID(i),
				Status: influxdb.Active,
			})

			if err != nil {
				t.Fatal(err)
			}
		}
	}

	tt := []struct {
		name    string
		setup   func(*testing.T, *Store, kv.Tx)
		update  func(*testing.T, *Store, kv.Tx)
		results func(*testing.T, *Store, kv.Tx)
	}{
		{
			name:  "create",
			setup: setup,
			results: func(t *testing.T, store *Store, tx kv.Tx) {
				auths, err := store.ListAuthorizations(context.Background(), tx, influxdb.AuthorizationFilter{})
				if err != nil {
					t.Fatal(err)
				}

				if len(auths) != 10 {
					t.Fatalf("expected 10 authorizations, got: %d", len(auths))
				}

				expected := []*influxdb.Authorization{}
				for i := 1; i <= 10; i++ {
					expected = append(expected, &influxdb.Authorization{
						ID:     influxdb.ID(i),
						Token:  fmt.Sprintf("randomtoken%d", i),
						OrgID:  influxdb.ID(i),
						UserID: influxdb.ID(i),
						Status: "active",
						Type:   influxdb.AuthorizationTypePlain,
					})
				}
				if !reflect.DeepEqual(auths, expected) {
					t.Fatalf("expected identical authorizations: \n%+v\n%+v", auths, expected)
				}

				// should not be able to create two authorizations with identical tokens
				err = store.CreateAuthorization(context.Background(), tx, &influxdb.Authorization{
					ID:     influxdb.ID(1),
					Token:  fmt.Sprintf("randomtoken%d", 1),
					OrgID:  influxdb.ID(1),
					UserID: influxdb.ID(1),
				})
				if err == nil {
					t.Fatalf("expected to be unable to create authorizations with identical tokens")
				}
			},
		},
		{
			name:  "read",
			setup: setup,
			results: func(t *testing.T, store *Store, tx kv.Tx) {
				for i := 1; i <= 10; i++ {
					expectedAuth := &influxdb.Authorization{
						ID:     influxdb.ID(i),
						Token:  fmt.Sprintf("randomtoken%d", i),
						OrgID:  influxdb.ID(i),
						UserID: influxdb.ID(i),
						Status: influxdb.Active,
						Type:   influxdb.AuthorizationTypePlain,
					}

					authByID, err := store.GetAuthorizationByID(context.Background(), tx, influxdb.ID(i))
					if err != nil {
						t.Fatalf("Unexpectedly could not acquire Authorization by ID [Error]: %v", err)
					}

					if !reflect.DeepEqual(authByID, expectedAuth) {
						t.Fatalf("ID TEST: expected identical authorizations:\n[Expected]: %+#v\n[Got]: %+#v", expectedAuth, authByID)
					}

					authByToken, err := store.GetAuthorizationByToken(context.Background(), tx, fmt.Sprintf("randomtoken%d", i))
					if err != nil {
						t.Fatalf("cannot get authorization by Token [Error]: %v", err)
					}

					if !reflect.DeepEqual(authByToken, expectedAuth) {
						t.Fatalf("TOKEN TEST: expected identical authorizations:\n[Expected]: %+#v\n[Got]: %+#v", expectedAuth, authByToken)
					}
				}

			},
		},
		{
			name:  "update",
			setup: setup,
			update: func(t *testing.T, store *Store, tx kv.Tx) {
				for i := 1; i <= 10; i++ {
					auth, err := store.GetAuthorizationByID(context.Background(), tx, influxdb.ID(i))
					if err != nil {
						t.Fatalf("Could not get authorization [Error]: %v", err)
					}

					auth.Status = influxdb.Inactive

					_, err = store.UpdateAuthorization(context.Background(), tx, influxdb.ID(i), auth)
					if err != nil {
						t.Fatalf("Could not get updated authorization [Error]: %v", err)
					}
				}
			},
			results: func(t *testing.T, store *Store, tx kv.Tx) {

				for i := 1; i <= 10; i++ {
					auth, err := store.GetAuthorizationByID(context.Background(), tx, influxdb.ID(i))
					if err != nil {
						t.Fatalf("Could not get authorization [Error]: %v", err)
					}

					expectedAuth := &influxdb.Authorization{
						ID:     influxdb.ID(i),
						Token:  fmt.Sprintf("randomtoken%d", i),
						OrgID:  influxdb.ID(i),
						UserID: influxdb.ID(i),
						Status: influxdb.Inactive,
						Type:   influxdb.AuthorizationTypePlain,
					}

					if !reflect.DeepEqual(auth, expectedAuth) {
						t.Fatalf("expected identical authorizations:\n[Expected] %+#v\n[Got] %+#v", expectedAuth, auth)
					}
				}
			},
		},
		{
			name:  "delete",
			setup: setup,
			update: func(t *testing.T, store *Store, tx kv.Tx) {
				for i := 1; i <= 10; i++ {
					err := store.DeleteAuthorization(context.Background(), tx, influxdb.ID(i))
					if err != nil {
						t.Fatalf("Could not delete authorization [Error]: %v", err)
					}
				}
			},
			results: func(t *testing.T, store *Store, tx kv.Tx) {
				for i := 1; i <= 10; i++ {
					_, err := store.GetAuthorizationByID(context.Background(), tx, influxdb.ID(i))
					if err == nil {
						t.Fatal("Authorization was not deleted correctly")
					}
				}
			},
		},
	}

	for _, testScenario := range tt {
		t.Run(testScenario.name, func(t *testing.T) {
			store := inmem.NewKVStore()
			if err := all.Up(context.Background(), zaptest.NewLogger(t), store); err != nil {
				t.Fatal(err)
			}

			ts, err := NewStore(store)
			if err != nil {
				t.Fatal(err)
			}

			// setup
			if testScenario.setup != nil {
				err := ts.Update(context.Background(), func(tx kv.Tx) error {
					testScenario.setup(t, ts, tx)
					return nil
				})

				if err != nil {
					t.Fatal(err)
				}
			}

			// update
			if testScenario.update != nil {
				err := ts.Update(context.Background(), func(tx kv.Tx) error {
					testScenario.update(t, ts, tx)
					return nil
				})

				if err != nil {
					t.Fatal(err)
				}
			}

			// results
			if testScenario.results != nil {
				err := ts.View(context.Background(), func(tx kv.Tx) error {
					testScenario.results(t, ts, tx)
					return nil
				})

				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func Test_filterAuthorizationsFn(t *testing.T) {
	var (
		otherID   = influxdb.ID(999)
		otherType = influxdb.AuthorizationTypeV1User
	)

	auth := influxdb.Authorization{
		ID:     1000,
		Token:  "foo",
		Status: influxdb.Active,
		Type:   influxdb.AuthorizationTypePlain,
		OrgID:  2000,
		UserID: 3000,
	}
	tests := []struct {
		name string
		filt influxdb.AuthorizationFilter
		auth influxdb.Authorization
		exp  bool
	}{
		{
			name: "default is true",
			filt: influxdb.AuthorizationFilter{},
			auth: auth,
			exp:  true,
		},
		{
			name: "match id",
			filt: influxdb.AuthorizationFilter{
				ID: &auth.ID,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "no match id",
			filt: influxdb.AuthorizationFilter{
				ID: &otherID,
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "match token",
			filt: influxdb.AuthorizationFilter{
				Token: &auth.Token,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "no match token",
			filt: influxdb.AuthorizationFilter{
				Token: pointer.String("2"),
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "match org",
			filt: influxdb.AuthorizationFilter{
				OrgID: &auth.OrgID,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "no match org",
			filt: influxdb.AuthorizationFilter{
				OrgID: &otherID,
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "match user",
			filt: influxdb.AuthorizationFilter{
				UserID: &auth.UserID,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "no match user",
			filt: influxdb.AuthorizationFilter{
				UserID: &otherID,
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "match org and user",
			filt: influxdb.AuthorizationFilter{
				OrgID:  &auth.OrgID,
				UserID: &auth.UserID,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "no match org and user",
			filt: influxdb.AuthorizationFilter{
				OrgID:  &otherID,
				UserID: &auth.UserID,
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "match type",
			filt: influxdb.AuthorizationFilter{
				Type: &auth.Type,
			},
			auth: auth,
			exp:  true,
		},
		{
			name: "match empty type as plain",
			filt: influxdb.AuthorizationFilter{
				Type: influxdb.AuthorizationTypePlain.Ptr(),
			},
			auth: influxdb.Authorization{Type: ""},
			exp:  true,
		},
		{
			name: "no match empty type as other",
			filt: influxdb.AuthorizationFilter{
				Type: &otherType,
			},
			auth: influxdb.Authorization{Type: ""},
			exp:  false,
		},
		{
			name: "no match type",
			filt: influxdb.AuthorizationFilter{
				Type: &otherType,
			},
			auth: auth,
			exp:  false,
		},
		{
			name: "no match org and type",
			filt: influxdb.AuthorizationFilter{
				OrgID: &auth.OrgID,
				Type:  &otherType,
			},
			auth: auth,
			exp:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pred := filterAuthorizationsFn(tc.filt)
			got := pred(&tc.auth)
			assert.Equal(t, tc.exp, got)
		})
	}
}
