
# Lake

Goals:
- Make-like syntax
- "pure", only uses executables and libraries through external dependencies
- Write scripts to generate files
- Write scripts to add programs to a "store"
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

```makefile
busybox_tar := download_file("http://lake.com/busybox.tar.gz")

store busybox_store: busybox_tar ./script.sh
    #!{{busybox_tar}}/bin/busybox sh
    sh ./script.sh
# or
store busybox_store(shell=["{{busybox_tar}}/bin", "sh"]): busybox_tar
    # Sets environment variables:
    # busybox_tar=/store/adfih9p8as/
    # store_imports=busybox_tar
    sh ./script.sh

busybox: busybox_store
    #!{{busybox_store}}/bin/busybox sh
    $busybox_store $@

busybox_shell: busybox_store
    #!{{busybox_tar}}/bin/busybox sh
    export PATH=$busybox_store/bin/
    sh $@
```


./Lakefile

```makefile
from "github.com/maxmcd/lake/busybox" import busybox_shell

set shell = [busybox_shell]

say_hello:
    echo "hello $1"
```

```bash
$ lake say_hello max
hello max
$ cd ..
$ lake lake/say_hello max
hello max
```




# Stories


## Generate a file using echo

```makefile
./bar.txt:
    echo "bar" > ./bar.txt

./foo.txt: ./bar.txt
    car bar.txt > ./foo.txt
    echo "foo" >> ./foo.txt
```

We run this in a separate environment when running. A folder is created like so:
`/tmp/build_location/`, any necessary source files are copied into the build folder.

When building `bar.txt` no source files are listed. We'll start the build in an empty directory like `/tmp/build_location/Ze7aef8aefaeta2que5ingeelohuich2`. Executables will be made available for the build (more on that later) and the script will be run.

Once that is built we can build `foo.txt`. A similar random folder is created but `./bar.txt` is copied into the build environment.


## Download a file and use it as an executable

```makefile
busybox_tar := download_file("http://lake.com/busybox.tar.gz")

store busybox_store: busybox_tar ./script.sh
    #!{{busybox_tar}}/bin/busybox sh
    sh ./script.sh
```

`download_file` takes a url and will attempt to automatically unarchive it. we can then directly call the enclosed files as executables

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

```makefile
busybox_tar := download_file("http://lake.com/busybox.tar.gz")


store busybox_store: busybox_tar ./script.sh
    #!{{busybox_tar}}/bin/busybox sh
    sh ./script.sh
```


## Import from another lakefile

```makefile
busybox_shell: busybox_store
    #!{{busybox_tar}}/bin/busybox sh
    export PATH=$busybox_store/bin/
    sh $@
```


```makefile
from proj import busybox_shell

say_hi: busybox_shell
    ${busybox_shell} -c 'echo yo'
```




# Laws to question

- Must run things in sandbox or processes will look at files and break reproducability
- Must copy files into sandbox, can't reference local files? (overlay fs?)
- Build results must go into $out folder (not with makefile pattern, at least with file generation use cases...)


How do commands work? how do arguments work?
