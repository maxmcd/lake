set -e
$busybox_tar/busybox-x86_64 mkdir $out/bin
$busybox_tar/busybox-x86_64 cp $busybox_tar/busybox-x86_64 $out/bin/busybox
cd $out/bin
for command in $(./busybox --list); do
    ./busybox ln -s busybox $command
done
