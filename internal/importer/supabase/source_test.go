package supabase

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/portable"
)

func TestDetectStoragePreflight(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hasStorage bool
		objects    []portable.Object
	}{
		{"one object", true, []portable.Object{{Bucket: "avatars", Key: "a.png"}}},
		{"objects", true, []portable.Object{{Bucket: "avatars", Key: "dir/a.png"}, {Bucket: "docs", Key: "b.pdf"}}},
		{"empty", true, nil},
		{"absent", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := detectionDB(t, tc.hasStorage, tc.objects)
			d, err := detect(context.Background(), db, importer.Input{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d.Objects, tc.objects) {
				t.Fatalf("detection objects = %+v, want %+v", d.Objects, tc.objects)
			}
			m := portable.Manifest{Storage: portable.Storage{Objects: d.Objects}}
			err = load.StoragePreflight(m, nil, false)
			if len(tc.objects) > 0 {
				var coded *errs.CLIError
				if !errors.As(err, &coded) || coded.Code != "E529" {
					t.Fatalf("missing target store: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if err := load.StoragePreflight(m, nil, true); err != nil {
				t.Fatalf("skip storage: %v", err)
			}
		})
	}
}

// detectionDB serves read-only detection queries over an in-memory PostgreSQL connection.
// Inputs: catalog presence and object identities. Outputs: pgx connection. No external database.
func detectionDB(t *testing.T, hasStorage bool, objects []portable.Object) *pgx.Conn {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	go func() {
		b := pgproto3.NewBackend(server, server)
		if _, err := b.ReceiveStartupMessage(); err != nil {
			return
		}
		b.Send(&pgproto3.AuthenticationOk{})
		b.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
		b.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
		b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		if b.Flush() != nil {
			return
		}
		for {
			msg, err := b.Receive()
			if err != nil {
				return
			}
			q, ok := msg.(*pgproto3.Query)
			if !ok {
				return
			}
			values := [][]string{}
			columns := 1
			oid := uint32(25)
			switch {
			case q.String == "SHOW server_version":
				values = [][]string{{"16.4"}}
			case strings.Contains(q.String, "server_version_num"):
				oid = 23
				values = [][]string{{"160004"}}
			case strings.Contains(q.String, "information_schema.columns"):
				oid = 16
				values = [][]string{{"t"}}
			case strings.Contains(q.String, "FROM pg_namespace"):
				values = [][]string{{"public"}}
			case strings.Contains(q.String, "FROM auth.users"):
				oid = 20
				values = [][]string{{"0"}}
			case strings.Contains(q.String, "CASE WHEN to_regclass"):
				oid = 20
				// Original detection only counted objects and discarded their identities.
				values = [][]string{{"0"}}
				if len(objects) > 0 {
					values = [][]string{{strconv.Itoa(len(objects))}}
				}
			case strings.Contains(q.String, "to_regclass('storage.objects')"):
				oid = 16
				v := "f"
				if hasStorage {
					v = "t"
				}
				values = [][]string{{v}}
			case strings.Contains(q.String, "FROM storage.objects") && hasStorage:
				columns = 2
				for _, o := range objects {
					values = append(values, []string{o.Bucket, o.Key})
				}
			default:
				b.Send(&pgproto3.ErrorResponse{Severity: "ERROR", Code: "42P01", Message: "unexpected query"})
				b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
				if b.Flush() != nil {
					return
				}
				continue
			}
			fields := make([]pgproto3.FieldDescription, columns)
			for i := range fields {
				fields[i] = pgproto3.FieldDescription{Name: []byte("value"), DataTypeOID: oid, DataTypeSize: -1}
			}
			b.Send(&pgproto3.RowDescription{Fields: fields})
			for _, row := range values {
				data := make([][]byte, len(row))
				for i, v := range row {
					data[i] = []byte(v)
				}
				b.Send(&pgproto3.DataRow{Values: data})
			}
			b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT")})
			b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if b.Flush() != nil {
				return
			}
		}
	}()
	cfg, err := pgx.ParseConfig("postgres://fixture@localhost/fixture?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DialFunc = func(context.Context, string, string) (net.Conn, error) { return client, nil }
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}
