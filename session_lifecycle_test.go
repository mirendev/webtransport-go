package webtransport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
)

func TestServerReleasesClosedSessions(t *testing.T) {
	for _, closeConnection := range []bool{false, true} {
		t.Run(fmt.Sprintf("closeConnection=%t", closeConnection), func(t *testing.T) {
			server := &Server{H3: http3.Server{TLSConfig: TLSConf}}
			server.H3.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := server.Upgrade(w, r)
				if err != nil {
					t.Errorf("upgrade: %v", err)
				}
			})
			socket := newUDPConnLocalhost(t)
			go server.Serve(socket)
			t.Cleanup(func() { _ = server.Close() })
			var conn *quic.Conn
			dialer := &Dialer{
				TLSClientConfig: &tls.Config{RootCAs: CertPool, ServerName: "localhost"},
				DialAddr: func(ctx context.Context, addr string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
					var err error
					conn, err = quic.DialAddr(ctx, addr, tlsConfig, config)
					return conn, err
				},
			}
			for i := 0; i < 10; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, session, err := dialer.Dial(ctx, "https://"+socket.LocalAddr().String()+"/", nil)
				cancel()
				require.NoError(t, err)
				dialConn := conn
				t.Cleanup(func() { _ = dialConn.CloseWithError(0, "test cleanup") })
				if closeConnection {
					require.NoError(t, conn.CloseWithError(0, "test"))
				} else {
					require.NoError(t, session.CloseWithError(0, "test"))
				}
				require.Eventually(t, func() bool {
					server.conns.mx.Lock()
					defer server.conns.mx.Unlock()
					return len(server.conns.conns) == 0
				}, time.Second, time.Millisecond, "server retained a closed session")
				require.Eventually(t, func() bool {
					dialer.conns.mx.Lock()
					defer dialer.conns.mx.Unlock()
					return len(dialer.conns.conns) == 0
				}, time.Second, time.Millisecond, "client retained a closed session")
				require.NoError(t, conn.CloseWithError(0, "test cleanup"))
			}
			require.NoError(t, dialer.Close())
		})
	}
}
