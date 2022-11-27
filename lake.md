
# Lake

Goals:
- Hcl config syntax
- "pure", only uses executables and libraries through external dependencies
- Write scripts to generate files
- Write scripts to add programs to a "store"
- File generation a first class pattern (like make)
- Build docker images
- Cache directory
- Compile a Go program and pull dependencies without the network
- Store dependencies and hashes/lock data
- Import other lake files and manage many lake files in many folders within one project
- Building across different platforms


./busybox/script.sh

```sh
set -e
$busybox_tar/busybox-x86_64 mkdir $out/bin
$busybox_tar/busybox-x86_64 cp $busybox_tar/busybox-x86_64 $out/bin/busybox
cd $out/bin
for command in $(./busybox --list); do
    ./busybox ln -s busybox $command
done
```

./busybox/Lakefile

```hcl
# builtin that downloads the file
store "busybox_tar" {
  env = {
    fetch_url = "true"
    url = "http://lake.com/busybox.tar.gz"
  }
  network = true
}

# Set the default shell to be the file that's in the download
config {
  shell = ["${busybox_tar}/busybox-x86_64", "sh"]
}

# Uses script.sh to build a store
store "busybox_store" {
  inputs = [busybox_tar, "./script.sh"]
  script = "sh ./script.sh"
}

# Surface an "ls" command using busybox
# Invoke with `lake ls ../`
target "ls" {
  inputs = [busybox_store]
  script = <<EOH
    $busybox_store/bin/ls $@
  EOH
}

target "busybox" {
  inputs = [busybox_store]
  script = "$busybox_store/bin/busybox $@"
}

shell = ["${busybox_store}/bin/sh"]
```


./Lakefile

```hcl
import "github.com/maxmcd/lake/lib/busybox" {}

config { shell = busybox.shell }

target "say_hello" {
  script = <<EOH
    echo "hello $1"
  EOH
}


target "./one.txt" {
  script = <<EOH
    echo 1 > ./one.txt
  EOH
}
```

```bash
$ lake say_hello max
hello max
$ lake ./one.txt
$ cat ./one.txt
1
```


# Stories


## Use a target as a command

```hcl
target "ls" {
  inputs = [busybox_store]
  script = <<EOH
    $busybox_store/bin/ls $@
  EOH
}
```

This can then be run like so:
```bash
$ ls -lah ./lib
total 0
drwxr-xr-x   3 max  staff    96B Nov 26 22:34 .
drwxr-xr-x  15 max  staff   480B Nov 26 22:22 ..
drwxr-xr-x   3 max  staff    96B Nov 26 22:34 busybox
```

Behind the scenes this (might):

1. Take the target and write it to a file in the store (like `.../store/a7sdfas78df.script`).
2. When `lake ls` is invoked, run an `exec` with the provided shell/interpreter and the script passed as the first argument. (do we pass args correctly?)
3. The script file is located in the store, but the command is executed from the position of the `lake` invocation.

We can also use target commands in another recipe:

```hcl
target "./this_file" {
  inputs = [ls]
  script = <<EOH
    ${ls} -lah > this_file
  EOH
}
```
In this situation we must provide a single-string value to replace `${ls}` with so that when it is invoked it executes the expected command within the correct context.

Do we create another file in the `.../store` with all of the expected environment variables loaded up? Maybe something like this:

```bash
export busybox_store="../store/asdfasdfasdfas/"
exec .../store/a7sdfas78df.script
```

But this doesn't work, because we can't assume the shell context is the same. So what is `${ls}` replaced with? Somewhat sensible might that is expands to `$interpreter $script_location`, but then how do we inject the necessary environment variables?

Do we inject a static executable `$hydrator $script-config` and then this thing runs an `exec` with the values included? Would be great to avoid injecting an executable. How else do we invoke `exec` in an environment-agnostic way?

## Generate a file using echo

```hcl
target "./bar.txt" {
  script = "echo 'bar' > ./bar.txt"
}
target "./foo.txt" {
  inputs = ["./bar.txt"]
  script = <<EOH
    car bar.txt > ./foo.txt
    echo "foo" >> ./foo.txt
  EOH
}
```

We run this in a separate environment when running. A folder is created like so:
`/tmp/build_location/`, any necessary source files are copied into the build folder.

When building `bar.txt` no source files are listed. We'll start the build in an empty directory like `/tmp/build_location/Ze7aef8aefaeta2que5ingeelohuich2`. Executables will be made available for the build (more on that later) and the script will be run.

Once that is built we can build `foo.txt`. A similar random folder is created but `./bar.txt` is copied into the build environment.


## Lakefile namespace is shared across files in a directory

**./download.Lakefile**

```hcl
# builtin that downloads the file
store "busybox_tar" {
  env = {
    fetch_url = "true"
    url = "http://lake.com/busybox.tar.gz"
  }
  network = true
}

# Set the default shell to be the file that's in the download
config {
  shell = ["${busybox_tar}/bin/busybox", "sh"]
}

```

**./foo.Lakefile**

```hcl
# Can reference busybox_tar from download.Lakefile
store "busybox_store" {
  inputs = [busybox_tar, "./script.sh"]
  script = "sh ./script.sh"
}
```


## Lakefile config sets the default shell


```hcl
config {
  shell = ["${busybox_tar}/bin/busybox", "sh"]
}

# This inherits the inputs and shell from the config
# TODO: inputs are detected? Hmm, is that a break from the norm?
target "say hi" { script = "echo hi" }

```

## Download a file and use it as an executable

```hcl
store "busybox_tar" {
  env = {
    fetch_url = "true"
    url = "http://lake.com/busybox.tar.gz"
  }
  network = true
}

# Uses script.sh to build a store
store "busybox_store" {
  inputs = [busybox_tar, "./script.sh"]
  script = "sh ./script.sh"
}
```

The store recipe with a `fetch_url` set to true and a `url` is a special store that will use the network to download a file.
Once downloaded, other recipes can then directly reference the the enclosed files.

## Build downloaded files and output their results

./busybox/script.sh

```sh
set -e
$busybox_tar/busybox-x86_64 mkdir $out/bin
$busybox_tar/busybox-x86_64 cp $busybox_tar/busybox-x86_64 $out/bin/busybox
cd $out/bin
for command in $(./busybox --list); do
    ./busybox ln -s busybox $command
done
```

./busybox/Lakefile

```hcl
store "busybox_tar" {
  env = {
    fetch_url = "true"
    url = "http://lake.com/busybox.tar.gz"
  }
  network = true
}

# Uses script.sh to build a store
store "busybox_store" {
  inputs = [busybox_tar, "./script.sh"]
  script = "sh ./script.sh"
}
```

Stores put their outputs in an $out directory. Alternatively file generation recipes just update the expected output at the expected location.

## Import from another lakefile


**./foo/Lakefile**
```hcl
hello = "hello"

_no = "no"

target "./foo" {}
```

**./bar/Lakefile**
```hcl
import "github.com/maxmcd/lib/foo" {}

hello_max = "${foo.hello} max"

# These don't work, can't import underscore names or file targets
# error = foo._no
# error = foo../foo
```

**./baz/Lakefile**
```hcl
import "github.com/maxmcd/lib/foo" { alias = "thing" }

# Names cannot conflict with import names, use an alias
store "foo" {}

hello_max = "${thing.hello} max"
```

## Use a target/command as an input to a store and reference it within the build script

```hcl
target "ls" {
  inputs = [busybox_store]
  script = <<EOH
    $busybox_store/bin/ls $@
  EOH
}

target "./this_file" {
  inputs = [ls]
  script = <<EOH
    ${ls} -lah > this_file
  EOH
}
```

## Generate a directory

## Use a glob pattern or directory as an input

## Use a negative glob pattern to exclude files?

## Use the network

## Use a cache directory

```hcl
store "go_binary" {
  inputs = ["..."]
  env = {
    # This is simple enough, and it means its a value that can be passed
    # around. Do we end up stripping the cache dir from the input tho?
    CACHE_DIR = cache_directory()
  }
  script = <<EOH
    go build -o $out/bin/thing
  EOH
}
```

# Notes

## Project setup

- Need a sum file for download hashes
- Otherwise we could use gopath-style dependency tree?
- lake.hcl project file?
- dependencies are named and configured centrally? what about nearby directories?
- No relative imports would be nice



## Laws to question

- Must run things in sandbox or processes will look at files and break reproducibility
- Must copy files into sandbox, can't reference local files? (overlay fs?)
- Build results must go into $out folder (not with makefile pattern, at least with file generation use cases...)


How do commands work? how do arguments work?
