# Data Formats

Pyrite ships first-version `json` and `xml` modules. The public standard
library is written in importable `.pyr` files.

`xml` is implemented in Pyrite using string methods. `json` is implemented in
Pyrite on top of compiler intrinsics for scalar conversion, array parsing, and
flat extraction. The runtime now has dictionaries, but the JSON module still
returns scalar `any` values and `list[any]` arrays rather than materializing
JSON objects as `dict` values.

## JSON

```pyrite
import json

def main():
    values: list[any] = ["Ada", 42, 3.5, True]
    encoded = json.dumps_list(values)
    parsed: list[any] = json.loads_array(encoded)

    user_json = "{\"name\":\"Ada\",\"score\":42,\"active\":true}"
    name = json.get_string(user_json, "name")
    score = json.get_int(user_json, "score")
    active = json.get_bool(user_json, "active")

    print(encoded)
    print(f"{name} score {score} active {active}")
    return score
```

Current APIs:

- `json.dumps(value: any) -> string`
- `json.dumps_list(items: list[any]) -> string`
- `json.loads(value: string) -> any`
- `json.loads_array(value: string) -> list[any]`
- `json.get_string(value: string, key: string) -> string`
- `json.get_int(value: string, key: string) -> int`
- `json.get_float(value: string, key: string) -> float`
- `json.get_bool(value: string, key: string) -> bool`

This version supports scalar values, mixed arrays of scalars, and flat object
field extraction. Nested object materialization into runtime dictionaries is
still planned.

## XML

```pyrite
import xml

def main():
    user_xml = xml.element("name", "Ada")
    print(user_xml)
    print(xml.get_text(user_xml, "name"))
    print(xml.escape("Ada < Pyrite & friends"))
    return 0
```

Current APIs:

- `xml.escape(value: string) -> string`
- `xml.unescape(value: string) -> string`
- `xml.element(name: string, text_value: string) -> string`
- `xml.get_text(value: string, name: string) -> string`

This first version handles simple element creation and first matching tag text
extraction in Pyrite. Attributes, namespaces, and streaming parsers are not
implemented yet.
