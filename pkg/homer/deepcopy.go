/*
Copyright 2024 RajSingh.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package homer

import (
	"encoding/json"
	"reflect"
)

// DeepCopyInto is kept handwritten because controller-gen cannot generate a
// safe copy for interface-valued upstream Homer fields. The generated copier
// for Dashboard calls this method when it copies HomerConfig.
func (c *HomerConfig) DeepCopyInto(out *HomerConfig) {
	*out = *c
	out.Columns = deepCopyValue(c.Columns)
	out.UpdateIntervalMs = deepCopyValue(c.UpdateIntervalMs)
	out.footerValue = deepCopyValue(c.footerValue)

	if c.ConnectivityCheck != nil {
		out.ConnectivityCheck = new(bool)
		*out.ConnectivityCheck = *c.ConnectivityCheck
	}
	out.Stylesheet = deepCopyValue(c.Stylesheet)
	c.Hotkey.DeepCopyInto(&out.Hotkey)
	c.Defaults.DeepCopyInto(&out.Defaults)
	c.Colors.DeepCopyInto(&out.Colors)
	if c.Proxy.Headers != nil {
		out.Proxy.Headers = cloneAnyMap(c.Proxy.Headers)
	}
	if c.Proxy.RawFields != nil {
		out.Proxy.RawFields = cloneRawFields(c.Proxy.RawFields)
	}
	if c.Message.Mapping != nil {
		out.Message.Mapping = cloneAnyMap(c.Message.Mapping)
	}
	if c.Message.RawFields != nil {
		out.Message.RawFields = cloneRawFields(c.Message.RawFields)
	}
	if c.Hotkey.RawFields != nil {
		out.Hotkey.RawFields = cloneRawFields(c.Hotkey.RawFields)
	}
	if c.Links != nil {
		out.Links = make([]Link, len(c.Links))
		for index := range c.Links {
			out.Links[index] = c.Links[index]
			out.Links[index].RawFields = cloneRawFields(c.Links[index].RawFields)
		}
	}
	if c.Services != nil {
		out.Services = make([]Service, len(c.Services))
		for i := range c.Services {
			c.Services[i].DeepCopyInto(&out.Services[i])
		}
	}
	if c.RawFields != nil {
		out.RawFields = cloneRawFields(c.RawFields)
	}
	if c.presentFields != nil {
		out.presentFields = cloneRawFields(c.presentFields)
	}
}

// DeepCopy creates an independent copy of HomerConfig.
func (c *HomerConfig) DeepCopy() *HomerConfig {
	if c == nil {
		return nil
	}
	out := new(HomerConfig)
	c.DeepCopyInto(out)
	return out
}

// DeepCopyInto is handwritten for the same reason as HomerConfig.DeepCopyInto:
// updateIntervalMs is intentionally open-ended so it can match upstream
// Homer’s numeric, string, boolean, and null forms.
func (i *Item) DeepCopyInto(out *Item) {
	*out = *i
	out.UpdateIntervalMs = deepCopyValue(i.UpdateIntervalMs)

	if i.UseCredentials != nil {
		out.UseCredentials = new(bool)
		*out.UseCredentials = *i.UseCredentials
	}
	if i.Headers != nil {
		out.Headers = cloneAnyMap(i.Headers)
	}
	if i.SuccessCodes != nil {
		out.SuccessCodes = append([]int(nil), i.SuccessCodes...)
	}
	if i.Quick != nil {
		out.Quick = make([]QuickLink, len(i.Quick))
		for index := range i.Quick {
			out.Quick[index] = i.Quick[index]
			out.Quick[index].RawFields = cloneRawFields(i.Quick[index].RawFields)
		}
	}
	if i.Parameters != nil {
		out.Parameters = cloneStringMap(i.Parameters)
	}
	if i.NestedObjects != nil {
		out.NestedObjects = make(map[string]map[string]string, len(i.NestedObjects))
		for key, value := range i.NestedObjects {
			out.NestedObjects[key] = cloneStringMap(value)
		}
	}
	if i.ArrayObjects != nil {
		out.ArrayObjects = make(map[string][]map[string]string, len(i.ArrayObjects))
		for key, values := range i.ArrayObjects {
			if values == nil {
				continue
			}
			out.ArrayObjects[key] = make([]map[string]string, len(values))
			for index, value := range values {
				out.ArrayObjects[key][index] = cloneStringMap(value)
			}
		}
	}
	if i.RawFields != nil {
		out.RawFields = cloneRawFields(i.RawFields)
	}
}

// DeepCopy creates an independent copy of Item.
func (i *Item) DeepCopy() *Item {
	if i == nil {
		return nil
	}
	out := new(Item)
	i.DeepCopyInto(out)
	return out
}

// DeepCopyInto is handwritten because the open header/mapping values use
// interface-backed JSON values that controller-gen cannot copy safely.
func (p *ProxyConfig) DeepCopyInto(out *ProxyConfig) {
	*out = *p
	if p.Headers != nil {
		out.Headers = cloneAnyMap(p.Headers)
	}
	if p.RawFields != nil {
		out.RawFields = cloneRawFields(p.RawFields)
	}
}

func (p *ProxyConfig) DeepCopy() *ProxyConfig {
	if p == nil {
		return nil
	}
	out := new(ProxyConfig)
	p.DeepCopyInto(out)
	return out
}

func (m *MessageConfig) DeepCopyInto(out *MessageConfig) {
	*out = *m
	if m.Mapping != nil {
		out.Mapping = cloneAnyMap(m.Mapping)
	}
	if m.RawFields != nil {
		out.RawFields = cloneRawFields(m.RawFields)
	}
}

func (m *MessageConfig) DeepCopy() *MessageConfig {
	if m == nil {
		return nil
	}
	out := new(MessageConfig)
	m.DeepCopyInto(out)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = deepCopyValue(value)
	}
	return out
}

func cloneRawFields(in map[string]json.RawMessage) map[string]json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		if value == nil {
			out[key] = nil
			continue
		}
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

// deepCopyValue recursively copies the JSON/YAML-compatible values held by
// interface fields while retaining their concrete scalar types.
func deepCopyValue(value any) any {
	if value == nil {
		return nil
	}
	return deepCopyReflect(reflect.ValueOf(value)).Interface()
}

func deepCopyReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := deepCopyReflect(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(copy)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(deepCopyReflect(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result.SetMapIndex(
				deepCopyReflect(iter.Key()),
				deepCopyReflect(iter.Value()),
			)
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		reflect.Copy(result, value)
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(deepCopyReflect(value.Index(index)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(deepCopyReflect(value.Index(index)))
		}
		return result
	case reflect.Struct:
		// Interface fields originate from JSON/YAML and normally contain no
		// structs. Preserve ordinary exported struct values defensively.
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if result.Field(index).CanSet() && value.Field(index).CanInterface() {
				result.Field(index).Set(deepCopyReflect(value.Field(index)))
			}
		}
		return result
	default:
		return value
	}
}
