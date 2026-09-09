package webtransport_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"
)

const settingsEnableWebtransportDraft06 = 0x2b603742

func TestLegacyOptInDoesNotBypassModernRequirements(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings map[uint64]uint64
		want     string
	}{
		{"modern peer without partial resets", map[uint64]uint64{settingsWebTransportEnabled: 1, settingsEnableWebtransportDraft06: 1}, "server didn't enable QUIC stream reset partial delivery"},
		{"modern explicitly disabled", map[uint64]uint64{settingsWebTransportEnabled: 0, settingsEnableWebtransportDraft06: 1}, "server didn't enable WebTransport"},
		{"no WebTransport advertisement", map[uint64]uint64{}, "server didn't enable WebTransport"},
		{"invalid legacy value", map[uint64]uint64{settingsEnableWebtransportDraft06: 2}, "server didn't enable WebTransport"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ln, err := quic.ListenAddr("localhost:0", webtransport.TLSConf, &quic.Config{EnableDatagrams: true})
			require.NoError(t, err)
			defer ln.Close()
			requestSent := make(chan bool, 1)
			go func() {
				conn, err := ln.Accept(ctx)
				if err != nil {
					requestSent <- false
					return
				}
				defer conn.CloseWithError(0, "")
				str, err := conn.OpenUniStream()
				if err != nil {
					requestSent <- false
					return
				}
				tc.settings[settingDatagram] = 1
				tc.settings[settingExtendedConnect] = 1
				_, err = str.Write(appendSettingsFrame([]byte{0}, tc.settings))
				if err != nil {
					requestSent <- false
					return
				}
				_, err = conn.AcceptStream(ctx)
				requestSent <- err == nil
			}()
			tr := &webtransport.Transport{AllowLegacyDraft06: true, TLSClientConfig: &tls.Config{RootCAs: webtransport.CertPool, ServerName: "localhost"}}
			defer tr.Close()
			_, _, err = tr.Dial(ctx, "https://"+ln.Addr().String(), nil)
			require.ErrorContains(t, err, tc.want)
			require.False(t, <-requestSent, "incompatible peer received a CONNECT request")
		})
	}
}

func TestLegacyHandshakeSelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		allowLegacy   bool
		modern        bool
		partialResets bool
		wantProtocol  string
	}{
		{name: "legacy opt-in", allowLegacy: true, wantProtocol: "webtransport"},
		{name: "legacy peer with partial resets", allowLegacy: true, partialResets: true, wantProtocol: "webtransport"},
		{name: "modern preferred", allowLegacy: true, modern: true, partialResets: true, wantProtocol: "webtransport-h3"},
		{name: "legacy disabled by default"},
		{name: "legacy disabled even with partial resets", partialResets: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ln, err := quic.ListenAddr("localhost:0", webtransport.TLSConf, &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: tc.partialResets})
			require.NoError(t, err)
			defer ln.Close()
			requests := make(chan *http.Request, 1)
			settings := map[uint64]uint64{settingsEnableWebtransportDraft06: 1}
			if tc.modern {
				settings[settingsWebTransportEnabled] = 1
			}
			srv := &http3.Server{EnableDatagrams: true, AdditionalSettings: settings, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r
				w.WriteHeader(http.StatusOK)
				_ = http.NewResponseController(w).Flush()
				<-ctx.Done()
			})}
			defer srv.Close()
			served := make(chan struct{})
			go func() {
				defer close(served)
				conn, err := ln.Accept(ctx)
				if err != nil {
					return
				}
				_ = srv.ServeQUICConn(conn)
			}()
			tr := &webtransport.Transport{AllowLegacyDraft06: tc.allowLegacy, TLSClientConfig: &tls.Config{RootCAs: webtransport.CertPool, ServerName: "localhost"}}
			defer tr.Close()
			headers := http.Header{"X-Application": []string{"kept"}}
			_, sess, err := tr.Dial(ctx, "https://"+ln.Addr().String(), headers)
			if tc.wantProtocol == "" {
				if tc.partialResets {
					require.ErrorContains(t, err, "server didn't enable WebTransport")
				} else {
					require.ErrorContains(t, err, "stream reset partial delivery")
				}
				select {
				case <-requests:
					t.Fatal("legacy CONNECT sent without opt-in")
				default:
				}
			} else {
				require.NoError(t, err)
				defer sess.CloseWithError(0, "")
				select {
				case req := <-requests:
					require.Equal(t, http.MethodConnect, req.Method)
					require.Equal(t, tc.wantProtocol, req.Proto)
					require.Equal(t, "kept", req.Header.Get("X-Application"))
					if tc.modern {
						require.Empty(t, req.Header.Get("Sec-Webtransport-Http3-Draft02"))
					} else {
						require.Equal(t, "1", req.Header.Get("Sec-Webtransport-Http3-Draft02"))
					}
				case <-ctx.Done():
					t.Fatal("CONNECT not received")
				}
			}
			// Dial must clone caller headers before adding the draft header.
			require.Empty(t, headers.Get("Sec-Webtransport-Http3-Draft02"))
			cancel()
			require.NoError(t, tr.Close())
			require.NoError(t, srv.Close())
			<-served
		})
	}
}
