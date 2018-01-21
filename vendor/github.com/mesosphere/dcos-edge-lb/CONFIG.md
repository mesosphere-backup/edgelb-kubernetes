# Config

The config that Edge LB accepts may be YAML or JSON, but JSON is preferred because it is more commonly used and known.
Examples may be found [here](examples/config).

The config model and supporting code are generated from [swagger.yml](apiserver/spec/data/swagger.yml). A
helpful graphical visualization tool for swagger can be found at [editor.swagger.io](http://editor.swagger.io)

## FAQ

### How do I convert YAML to JSON?

`dcos edgelb config --to-json=/path/to/json`

### How do I convert JSON to YAML?

There currently isn't an automated way to do this, the suggested method is to
hand convert it, and then use the YAML to JSON conversion on your YAML and
then do a diff between your YAML and the JSON.

```
# First hand convert JSON to YAML
# Then compare to original json with the `diff` shell command.
# Here we "convert" even the JSON file to get consistently formatted JSON
diff <(dcos edgelb config --to-json=myconfig.yaml) <(dcos edgelb config --to-json=myconfig.json)
```

### How do I create an empty object in YAML?

The syntax is actually the same as in JSON. For example, basic use of `sticky`
involves setting it to the empty object.

YAML:
```
  sticky: {}
```

JSON:
```
{
  "sticky": {}
}
```

### Where does JSON expect commas?

You put them everywhere except after the last element in an object or array.

```
{
  "key1": "value1",
  "key2": "value2"
}
```

```
{
  "key": ["value1", "value2"]
}
```
