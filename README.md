
**Lakefile**

```hcl
busybox_tar = download_file("http://lake.com/busybox.tar.gz")

config {
  shell = ["${busybox_tar}/bin/busybox", "sh"]
}

store "empty_store" {
  inputs = []
  script = <<EOH
    sh ./script.sh
  EOH
}

store "busybox_store" {
  inputs = [busybox_tar, "./script.sh"]
  script = <<EOH
    #!${busybox_tar}/bin/busybox sh
    sh ./script.sh
  EOH
}

store "busybox_store_alt" {
  inputs = [busybox_tar, "./script.sh"]
  shell  = ["${busybox_tar}/bin/busybox", "sh"]
  script = "sh ./script.sh"
}

target "busybox" {
  inputs = [busybox_store]
  script = <<EOH
    #!${busybox_store}/bin/busybox sh
    $busybox_store $@
  EOH
}
```

**Output**

```json
{
  "Stores": [
    {
      "Name": "busybox_store",
      "Inputs": [
        "http://lake.com/busybox.tar.gz",
        "./script.sh"
      ],
      "Script": "    #!http://lake.com/busybox.tar.gz/bin/busybox sh\n    sh ./script.sh\n",
      "Shell": null
    },
    {
      "Name": "busybox_store_alt",
      "Inputs": [
        "http://lake.com/busybox.tar.gz",
        "./script.sh"
      ],
      "Script": "sh ./script.sh",
      "Shell": [
        "http://lake.com/busybox.tar.gz/bin/busybox",
        "sh"
      ]
    },
    {
      "Name": "empty_store",
      "Inputs": [],
      "Script": "    sh ./script.sh\n",
      "Shell": null
    }
  ],
  "Configs": [
    {
      "Shell": [
        "http://lake.com/busybox.tar.gz/bin/busybox",
        "sh"
      ],
      "Temporary": ""
    }
  ],
  "Targets": [
    {
      "Name": "busybox",
      "Inputs": [
        "{{ 5a8139b2fa495f0b6a457d979e691436d651a281af5086289b45da2797308ca1 }}"
      ],
      "Script": "    #!{{ 5a8139b2fa495f0b6a457d979e691436d651a281af5086289b45da2797308ca1 }}/bin/busybox sh\n    $busybox_store $@\n",
      "Shell": null
    }
  ]
}
```
