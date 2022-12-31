# Notes

- Language file parsing and generating build definitions
 - shells?
 -
- Building (in order building, in a sandbox)
- End user tooling and ergonomics
    - How to run things
    - Project setup
    - External dependencies????



```json
{"78as69f8a76ds9fa86": {
    "shell": ["{ asdf6asdf69as8d76a }/busybox-x86_64", "sh"],
    "script": "echo hi",
    "dependencies": ["asdf6asdf69as8d76a", "/user/maxm/file"],
    "env" : { "Lake":"fish" },

    "network": false,
    "arch": "linux-x86",
}}

{"78as69f8a76ds9fa86": {
    "builder": "{ asdf6asdf69as8d76a }/busybox-x86_64",
    "args": ["sh" "-c" "echo hi", "/store/asfas9dyfasodf7896as89df6/magicfilename.sh"],
    "dependencies": ["asdf6asdf69as8d76a", "/user/maxm/file"],
    "env" : { "Lake":"fish" },

    "network": false,
    "arch": "linux-x86",
}}


    "build": "{ asdf6asdf69as8d76a }/python",
    "args": ["sh" "-c" "echo hi"],

```


```
bash -c "echo hi"

echo 'echo "hi"' > script.sh
bash ./script.sh
```

```hcl
store "go" {
  inputs = [go_src]
  shell  = ["${busybox_tar}/busybox-x86_64", "sh"]
  script = <<EOH
    cp -r ${go_src} ./src
    cd ./src/src
    sh ./build-complete.sh
    mkdir -p $out/bin $out/lib
    cp -r ./output/bin $out/bin
    cp -r ./output/lib $out/lib
  EOH
}

command "go_shell" {
  inputs = [go]
  shell  = ["${busybox_tar}/busybox-x86_64", "sh"]
  script = <<EOH
    PATH=$PATH:${go}/bin
    PATH=$PATH:${busybox_tar}/bin
    sh $@
  EOH
}

command "new_shell" {
  inputs = [go, bash, mysql, librdkafka]
  shell = stdlib.depComposer
}


shell "new_shell" {
  inputs
}

shell "go" {
  bin_path = "{go}/bin"
  ...
}

store "mything" {
  inputs = ["./*.go"]
  shell = ["${go_shell}"]
  script = "go build"
}

store "mything" {
  inputs = ["./*.go"]
  shell = ["${go}/bin/go", "build"]
  script = "."
}

store "busybox_store" {
  inputs = [busybox_tar, "./install.sh", ]
  shell  = ["${busybox_tar}/busybox-x86_64", "sh", "install.sh"]
}
```
