// Package ble talks to a Bluetooth Low Energy peripheral through BlueZ, the
// Linux Bluetooth stack, over D-Bus. It is what connects kupic to a Lumix
// camera before the camera brings its Wi-Fi up.
//
// Nothing here pairs or bonds: a Lumix camera needs no link-layer security,
// so a plain connection is enough (see lumix.go).
package ble

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// The BlueZ service and the interfaces of its GATT objects.
const (
	bluezService = "org.bluez"
	adapterIface = "org.bluez.Adapter1"
	deviceIface  = "org.bluez.Device1"
	charIface    = "org.bluez.GattCharacteristic1"
	propsIface   = "org.freedesktop.DBus.Properties"
	omIface      = "org.freedesktop.DBus.ObjectManager"
	rootPath     = dbus.ObjectPath("/")
)

// Device is a Bluetooth device the adapter has seen.
type Device struct {
	Path    dbus.ObjectPath
	Name    string
	Address string
	RSSI    int16
	// UUIDs are the services the device advertises.
	UUIDs []string
}

// Central is a Bluetooth Low Energy central, backed by the machine's
// adapter.
type Central struct {
	conn    *dbus.Conn
	adapter dbus.ObjectPath
}

// NewCentral opens a connection to BlueZ and finds the machine's adapter.
func NewCentral() (*Central, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("reach BlueZ: %w", err)
	}
	c := &Central{conn: conn}
	if err := c.findAdapter(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// Close releases the connection to BlueZ.
func (c *Central) Close() error { return c.conn.Close() }

// Adapter is the name of the adapter kupic drives, as "hci0".
func (c *Central) Adapter() string {
	return strings.TrimPrefix(string(c.adapter), "/org/bluez/")
}

// findAdapter picks the machine's first Bluetooth adapter.
func (c *Central) findAdapter() error {
	objects, err := c.managedObjects()
	if err != nil {
		return err
	}
	for path, ifaces := range objects {
		if _, ok := ifaces[adapterIface]; ok {
			c.adapter = path
			return nil
		}
	}
	return fmt.Errorf("this machine has no Bluetooth adapter")
}

// managedObjects returns every object BlueZ manages, with its interfaces.
func (c *Central) managedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	obj := c.conn.Object(bluezService, rootPath)
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := obj.Call(omIface+".GetManagedObjects", 0).Store(&objects); err != nil {
		return nil, fmt.Errorf("list what BlueZ manages: %w", err)
	}
	return objects, nil
}

// Scan looks for devices for d, returning those match accepts. Discovery
// runs until the time is up; the camera is not connected to.
func (c *Central) Scan(ctx context.Context, d time.Duration, match func(Device) bool) ([]Device, error) {
	adapter := c.conn.Object(bluezService, c.adapter)
	// A filter keeps the adapter to Bluetooth Low Energy, so the scan sees
	// the camera's advertisements and not a page of phones and headphones.
	filter := map[string]dbus.Variant{
		"Transport": dbus.MakeVariant("le"),
	}
	if err := adapter.Call(adapterIface+".SetDiscoveryFilter", 0, filter).Err; err != nil {
		return nil, fmt.Errorf("set the discovery filter: %w", err)
	}

	signals := make(chan *dbus.Signal, 64)
	c.conn.Signal(signals)
	defer c.conn.RemoveSignal(signals)
	if err := c.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(rootPath),
		dbus.WithMatchInterface(omIface),
		dbus.WithMatchMember("InterfacesAdded"),
	); err != nil {
		return nil, err
	}
	defer c.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(rootPath),
		dbus.WithMatchInterface(omIface),
		dbus.WithMatchMember("InterfacesAdded"),
	)

	if err := adapter.Call(adapterIface+".StartDiscovery", 0).Err; err != nil {
		return nil, fmt.Errorf("start scanning: %w", err)
	}
	defer adapter.Call(adapterIface+".StopDiscovery", 0)

	deadline := time.Now().Add(d)
	found := map[dbus.ObjectPath]Device{}
	collect := func(objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) {
		for path, ifaces := range objects {
			props, ok := ifaces[deviceIface]
			if !ok {
				continue
			}
			device := deviceFrom(path, props)
			if match(device) {
				found[path] = device
			}
		}
	}

	if objects, err := c.managedObjects(); err == nil {
		collect(objects)
	}

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			return devices(found), ctx.Err()
		case signal := <-signals:
			if signal.Name != omIface+".InterfacesAdded" || len(signal.Body) != 2 {
				continue
			}
			path, _ := signal.Body[0].(dbus.ObjectPath)
			ifaces, _ := signal.Body[1].(map[string]map[string]dbus.Variant)
			if props, ok := ifaces[deviceIface]; ok {
				device := deviceFrom(path, props)
				if match(device) {
					found[path] = device
				}
			}
		case <-time.After(remaining):
			break
		}
	}
	return devices(found), nil
}

// devices flattens what a scan found.
func devices(found map[dbus.ObjectPath]Device) []Device {
	out := make([]Device, 0, len(found))
	for _, d := range found {
		out = append(out, d)
	}
	return out
}

// deviceFrom reads a device's properties.
func deviceFrom(path dbus.ObjectPath, props map[string]dbus.Variant) Device {
	return Device{
		Path:    path,
		Name:    stringProp(props, "Name"),
		Address: stringProp(props, "Address"),
		RSSI:    int16Prop(props, "RSSI"),
		UUIDs:   stringsProp(props, "UUIDs"),
	}
}

// Conn is a live connection to a device.
type Conn struct {
	conn    *dbus.Conn
	device  dbus.ObjectPath
	adapter string
}

// Connect opens a connection to a device and resolves the GATT
// characteristics kupic writes to. The attempt is given up on when ctx
// ends, since BlueZ otherwise waits on the controller indefinitely.
func (c *Central) Connect(ctx context.Context, device Device) (*Conn, error) {
	// A connection attempt while the adapter is still scanning is aborted
	// by the controller, so discovery is stopped and given a moment first.
	c.stopDiscovery()
	time.Sleep(500 * time.Millisecond)

	obj := c.conn.Object(bluezService, device.Path)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := obj.CallWithContext(attemptCtx, deviceIface+".Connect", 0).Err
		cancel()
		if err == nil {
			conn := &Conn{conn: c.conn, device: device.Path, adapter: string(c.adapter)}
			if err := conn.waitForServices(ctx); err != nil {
				return nil, err
			}
			return conn, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("connect to %s: %w", displayName(device), lastErr)
}

// waitForServices waits until BlueZ has discovered the device's GATT
// database. A characteristic cannot be looked up before then, and the
// lookup would report an empty device.
func (c *Conn) waitForServices(ctx context.Context) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		props, err := c.properties(c.device, deviceIface)
		if err == nil && boolProp(props, "ServicesResolved") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return fmt.Errorf("the camera did not describe its services")
}

// stopDiscovery ends any scan the adapter is running.
func (c *Central) stopDiscovery() {
	c.conn.Object(bluezService, c.adapter).Call(adapterIface+".StopDiscovery", 0)
}

// Disconnect ends the connection.
func (c *Conn) Disconnect() error {
	obj := c.conn.Object(bluezService, c.device)
	return obj.Call(deviceIface+".Disconnect", 0).Err
}

// Connected reports whether the device is still connected.
func (c *Conn) Connected() bool {
	props, err := c.properties(c.device, deviceIface)
	if err != nil {
		return false
	}
	return boolProp(props, "Connected")
}

// Write sends data to a characteristic, addressed by its UUID.
func (c *Conn) Write(uuid string, data []byte) error {
	path, err := c.characteristic(uuid)
	if err != nil {
		return err
	}
	obj := c.conn.Object(bluezService, path)
	return obj.Call(charIface+".WriteValue", 0, data, map[string]dbus.Variant{}).Err
}

// Subscribe asks a characteristic to notify, calling fn with each value. The
// returned function stops the notifications.
func (c *Conn) Subscribe(uuid string, fn func([]byte)) (func(), error) {
	path, err := c.characteristic(uuid)
	if err != nil {
		return nil, err
	}
	signals := make(chan *dbus.Signal, 16)
	c.conn.Signal(signals)
	match := []dbus.MatchOption{
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(propsIface),
		dbus.WithMatchMember("PropertiesChanged"),
	}
	if err := c.conn.AddMatchSignal(match...); err != nil {
		c.conn.RemoveSignal(signals)
		return nil, err
	}
	if err := c.conn.Object(bluezService, path).Call(charIface+".StartNotify", 0).Err; err != nil {
		c.conn.RemoveMatchSignal(match...)
		c.conn.RemoveSignal(signals)
		return nil, fmt.Errorf("subscribe to %s: %w", uuid, err)
	}

	go func() {
		for signal := range signals {
			if len(signal.Body) < 2 {
				continue
			}
			changed, ok := signal.Body[1].(map[string]dbus.Variant)
			if !ok {
				continue
			}
			if value, ok := changed["Value"]; ok {
				if data, ok := value.Value().([]byte); ok {
					fn(data)
				}
			}
		}
	}()

	return func() {
		c.conn.Object(bluezService, path).Call(charIface+".StopNotify", 0)
		c.conn.RemoveMatchSignal(match...)
		c.conn.RemoveSignal(signals)
		close(signals)
	}, nil
}

// Flags returns the properties a characteristic supports, as BlueZ names
// them: "read", "write", "notify" and the like.
func (c *Conn) Flags(uuid string) ([]string, error) {
	path, err := c.characteristic(uuid)
	if err != nil {
		return nil, err
	}
	props, err := c.properties(path, charIface)
	if err != nil {
		return nil, err
	}
	return stringsProp(props, "Flags"), nil
}

// Services lists the GATT services the device exposes, as UUIDs.
func (c *Conn) Services() ([]string, error) {
	objects, err := c.managedObjects()
	if err != nil {
		return nil, err
	}
	prefix := string(c.device) + "/"
	var out []string
	for path, ifaces := range objects {
		if !strings.HasPrefix(string(path), prefix) {
			continue
		}
		if props, ok := ifaces["org.bluez.GattService1"]; ok {
			out = append(out, stringProp(props, "UUID"))
		}
	}
	return out, nil
}

// Characteristics lists the device's GATT characteristics, as UUIDs.
func (c *Conn) Characteristics() ([]string, error) {
	chars, err := c.characteristics()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(chars))
	for uuid := range chars {
		out = append(out, uuid)
	}
	return out, nil
}

// characteristic finds the object path of a characteristic by its UUID. UUIDs
// are stable across connections; handles are not, which is why BlueZ is
// asked by UUID.
func (c *Conn) characteristic(uuid string) (dbus.ObjectPath, error) {
	chars, err := c.characteristics()
	if err != nil {
		return "", err
	}
	path, ok := chars[strings.ToLower(uuid)]
	if !ok {
		return "", fmt.Errorf("the camera does not expose %s", uuid)
	}
	return path, nil
}

// characteristics maps each characteristic UUID under the device to its
// object path.
func (c *Conn) characteristics() (map[string]dbus.ObjectPath, error) {
	objects, err := c.managedObjects()
	if err != nil {
		return nil, err
	}
	prefix := string(c.device) + "/"
	out := map[string]dbus.ObjectPath{}
	for path, ifaces := range objects {
		if !strings.HasPrefix(string(path), prefix) {
			continue
		}
		if props, ok := ifaces[charIface]; ok {
			out[strings.ToLower(stringProp(props, "UUID"))] = path
		}
	}
	return out, nil
}

func (c *Conn) managedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	return (&Central{conn: c.conn}).managedObjects()
}

// properties reads an object's properties for one interface.
func (c *Conn) properties(path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	obj := c.conn.Object(bluezService, path)
	var props map[string]dbus.Variant
	if err := obj.Call(propsIface+".GetAll", 0, iface).Store(&props); err != nil {
		return nil, err
	}
	return props, nil
}

// displayName names a device in an error.
func displayName(d Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Address
}

func stringProp(props map[string]dbus.Variant, name string) string {
	if v, ok := props[name]; ok {
		if s, ok := v.Value().(string); ok {
			return s
		}
	}
	return ""
}

func stringsProp(props map[string]dbus.Variant, name string) []string {
	if v, ok := props[name]; ok {
		if s, ok := v.Value().([]string); ok {
			return s
		}
	}
	return nil
}

func int16Prop(props map[string]dbus.Variant, name string) int16 {
	if v, ok := props[name]; ok {
		if n, ok := v.Value().(int16); ok {
			return n
		}
	}
	return 0
}

func boolProp(props map[string]dbus.Variant, name string) bool {
	if v, ok := props[name]; ok {
		if b, ok := v.Value().(bool); ok {
			return b
		}
	}
	return false
}
