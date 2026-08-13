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
func (in *HomerConfig) DeepCopyInto(out *HomerConfig) {
	*out = *in
	out.Columns = deepCopyValue(in.Columns)
	out.UpdateIntervalMs = deepCopyValue(in.UpdateIntervalMs)
	out.footerValue = deepCopyValue(in.footerValue)

	if in.ConnectivityCheck != nil {
		out.ConnectivityCheck = new(bool)
		*out.ConnectivityCheck = *in.ConnectivityCheck
	}
	if in.Stylesheet != nil {
		out.Stylesheet = append([]string(nil), in.Stylesheet...)
	}
	in.Hotkey.DeepCopyInto(&out.Hotkey)
	in.Defaults.DeepCopyInto(&out.Defaults)
	in.Colors.DeepCopyInto(&out.Colors)
	if in.Proxy.Headers != nil {
		out.Proxy.Headers = cloneAnyMap(in.Proxy.Headers)
	}
	if in.Proxy.RawFields != nil {
		out.Proxy.RawFields = cloneRawFields(in.Proxy.RawFields)
	}
	if in.Message.Mapping != nil {
		out.Message.Mapping = cloneAnyMap(in.Message.Mapping)
	}
	if in.Message.RawFields != nil {
		out.Message.RawFields = cloneRawFields(in.Message.RawFields)
	}
	if in.Hotkey.RawFields != nil {
		out.Hotkey.RawFields = cloneRawFields(in.Hotkey.RawFields)
	}
	if in.Links != nil {
		out.Links = make([]Link, len(in.Links))
		for index := range in.Links {
			out.Links[index] = in.Links[index]
			out.Links[index].RawFields = cloneRawFields(in.Links[index].RawFields)
		}
	}
	if in.Services != nil {
		out.Services = make([]Service, len(in.Services))
		for i := range in.Services {
			in.Services[i].DeepCopyInto(&out.Services[i])
		}
	}
	if in.RawFields != nil {
		out.RawFields = cloneRawFields(in.RawFields)
	}
	if in.presentFields != nil {
		out.presentFields = cloneRawFields(in.presentFields)
	}
}

// DeepCopy creates an independent copy of HomerConfig.
func (in *HomerConfig) DeepCopy() *HomerConfig {
	if in == nil {
		return nil
	}
	out := new(HomerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is handwritten for the same reason as HomerConfig.DeepCopyInto:
// updateIntervalMs is intentionally open-ended so it can match upstream
// Homer’s numeric, string, boolean, and null forms.
func (in *Item) DeepCopyInto(out *Item) {
	*out = *in
	out.UpdateIntervalMs = deepCopyValue(in.UpdateIntervalMs)

	if in.UseCredentials != nil {
		out.UseCredentials = new(bool)
		*out.UseCredentials = *in.UseCredentials
	}
	if in.Headers != nil {
		out.Headers = cloneAnyMap(in.Headers)
	}
	if in.SuccessCodes != nil {
		out.SuccessCodes = append([]int(nil), in.SuccessCodes...)
	}
	if in.Quick != nil {
		out.Quick = make([]QuickLink, len(in.Quick))
		for index := range in.Quick {
			out.Quick[index] = in.Quick[index]
			out.Quick[index].RawFields = cloneRawFields(in.Quick[index].RawFields)
		}
	}
	if in.Parameters != nil {
		out.Parameters = cloneStringMap(in.Parameters)
	}
	if in.NestedObjects != nil {
		out.NestedObjects = make(map[string]map[string]string, len(in.NestedObjects))
		for key, value := range in.NestedObjects {
			out.NestedObjects[key] = cloneStringMap(value)
		}
	}
	if in.ArrayObjects != nil {
		out.ArrayObjects = make(map[string][]map[string]string, len(in.ArrayObjects))
		for key, values := range in.ArrayObjects {
			if values == nil {
				continue
			}
			out.ArrayObjects[key] = make([]map[string]string, len(values))
			for index, value := range values {
				out.ArrayObjects[key][index] = cloneStringMap(value)
			}
		}
	}
	if in.RawFields != nil {
		out.RawFields = cloneRawFields(in.RawFields)
	}
}

// DeepCopy creates an independent copy of Item.
func (in *Item) DeepCopy() *Item {
	if in == nil {
		return nil
	}
	out := new(Item)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is handwritten because the open header/mapping values use
// interface-backed JSON values that controller-gen cannot copy safely.
func (in *ProxyConfig) DeepCopyInto(out *ProxyConfig) {
	*out = *in
	if in.Headers != nil {
		out.Headers = cloneAnyMap(in.Headers)
	}
	if in.RawFields != nil {
		out.RawFields = cloneRawFields(in.RawFields)
	}
}

func (in *ProxyConfig) DeepCopy() *ProxyConfig {
	if in == nil {
		return nil
	}
	out := new(ProxyConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *MessageConfig) DeepCopyInto(out *MessageConfig) {
	*out = *in
	if in.Mapping != nil {
		out.Mapping = cloneAnyMap(in.Mapping)
	}
	if in.RawFields != nil {
		out.RawFields = cloneRawFields(in.RawFields)
	}
}

func (in *MessageConfig) DeepCopy() *MessageConfig {
	if in == nil {
		return nil
	}
	out := new(MessageConfig)
	in.DeepCopyInto(out)
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
