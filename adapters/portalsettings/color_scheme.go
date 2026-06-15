package portalsettings

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/bnema/tmux-session-sidebar/core/config"
)

const (
	portalBusName   = "org.freedesktop.portal.Desktop"
	portalObject    = "/org/freedesktop/portal/desktop"
	portalInterface = "org.freedesktop.portal.Settings"
	portalNamespace = "org.freedesktop.appearance"
	portalKey       = "color-scheme"
)

type ColorSchemeSource struct {
	Connect func() (*dbus.Conn, error)
}

func (s ColorSchemeSource) CurrentPreference(ctx context.Context) (config.SystemColorSchemePreference, error) {
	conn, err := s.connect()
	if err != nil {
		return config.SystemColorSchemeNoPreference, err
	}
	defer conn.Close()
	value, err := readPortalColorScheme(ctx, conn)
	if err != nil {
		return config.SystemColorSchemeNoPreference, err
	}
	return config.ParseSystemColorSchemePreference(value), nil
}

func (s ColorSchemeSource) Watch(ctx context.Context) (<-chan config.SystemColorSchemePreference, <-chan error, error) {
	conn, err := s.connect()
	if err != nil {
		return nil, nil, err
	}
	if err := conn.AddMatchSignalContext(ctx,
		dbus.WithMatchSender(portalBusName),
		dbus.WithMatchObjectPath(portalObject),
		dbus.WithMatchInterface(portalInterface),
		dbus.WithMatchMember("SettingChanged"),
		dbus.WithMatchArg0Namespace(portalNamespace),
		dbus.WithMatchArg(1, portalKey),
	); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	changes := make(chan config.SystemColorSchemePreference, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(changes)
		defer close(errs)
		defer conn.RemoveSignal(signals)
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				preference, ok, err := parseSettingChangedSignal(signal)
				if err != nil {
					select {
					case errs <- err:
					case <-ctx.Done():
					}
					continue
				}
				if !ok {
					continue
				}
				select {
				case changes <- preference:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return changes, errs, nil
}

func (s ColorSchemeSource) connect() (*dbus.Conn, error) {
	if s.Connect != nil {
		return s.Connect()
	}
	return dbus.ConnectSessionBus()
}

func readPortalColorScheme(ctx context.Context, conn *dbus.Conn) (uint32, error) {
	object := conn.Object(portalBusName, dbus.ObjectPath(portalObject))
	var value dbus.Variant
	if err := object.CallWithContext(ctx, portalInterface+".ReadOne", 0, portalNamespace, portalKey).Store(&value); err == nil {
		return variantUint32(value)
	}
	if err := object.CallWithContext(ctx, portalInterface+".Read", 0, portalNamespace, portalKey).Store(&value); err != nil {
		return 0, err
	}
	return variantUint32(value)
}

func parseSettingChangedSignal(signal *dbus.Signal) (config.SystemColorSchemePreference, bool, error) {
	if signal == nil || len(signal.Body) != 3 {
		return config.SystemColorSchemeNoPreference, false, nil
	}
	namespace, ok := signal.Body[0].(string)
	if !ok || namespace != portalNamespace {
		return config.SystemColorSchemeNoPreference, false, nil
	}
	key, ok := signal.Body[1].(string)
	if !ok || key != portalKey {
		return config.SystemColorSchemeNoPreference, false, nil
	}
	variant, ok := signal.Body[2].(dbus.Variant)
	if !ok {
		return config.SystemColorSchemeNoPreference, false, fmt.Errorf("portal setting changed signal carried %T, want dbus.Variant", signal.Body[2])
	}
	value, err := variantUint32(variant)
	if err != nil {
		return config.SystemColorSchemeNoPreference, false, err
	}
	return config.ParseSystemColorSchemePreference(value), true, nil
}

func variantUint32(variant dbus.Variant) (uint32, error) {
	value := variant.Value()
	for {
		switch typed := value.(type) {
		case uint32:
			return typed, nil
		case dbus.Variant:
			value = typed.Value()
		case uint:
			return uint32(typed), nil
		case int32:
			if typed < 0 {
				return 0, fmt.Errorf("portal color-scheme variant was negative: %d", typed)
			}
			return uint32(typed), nil
		case int:
			if typed < 0 {
				return 0, fmt.Errorf("portal color-scheme variant was negative: %d", typed)
			}
			return uint32(typed), nil
		default:
			return 0, fmt.Errorf("portal color-scheme variant carried %T, want uint32", value)
		}
	}
}
