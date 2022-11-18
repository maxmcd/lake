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
  env = {
    FOO = "bar"
  }
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
```json
{
  "Stores": [
    {
      "Name": "empty_store",
      "Inputs": [],
      "Script": "    sh ./script.sh\n",
      "Shell": null,
      "Env": null
    },
    {
      "Name": "busybox_store_alt",
      "Inputs": [
        "{{ 0704a8f2f52ceac487818c51755e87256acb4faf45b86d4fbda32064ddd3cc9d }}",
        "./script.sh"
      ],
      "Script": "sh ./script.sh",
      "Shell": [
        "{{ 0704a8f2f52ceac487818c51755e87256acb4faf45b86d4fbda32064ddd3cc9d }}/bin/busybox",
        "sh"
      ],
      "Env": null
    },
    {
      "Name": "busybox_store",
      "Inputs": [
        "{{ 0704a8f2f52ceac487818c51755e87256acb4faf45b86d4fbda32064ddd3cc9d }}",
        "./script.sh"
      ],
      "Script": "    #!{{ 0704a8f2f52ceac487818c51755e87256acb4faf45b86d4fbda32064ddd3cc9d }}/bin/busybox sh\n    sh ./script.sh\n",
      "Shell": null,
      "Env": {
        "FOO": "bar"
      }
    },
    {
      "Name": "download_file",
      "Inputs": null,
      "Script": "",
      "Shell": null,
      "Env": {
        "fetch_url": "true",
        "url": "http://lake.com/busybox.tar.gz"
      }
    }
  ],
  "Configs": [
    {
      "Shell": [
        "{{ 0704a8f2f52ceac487818c51755e87256acb4faf45b86d4fbda32064ddd3cc9d }}/bin/busybox",
        "sh"
      ],
      "Temporary": ""
    }
  ],
  "Targets": [
    {
      "Name": "busybox",
      "Inputs": [
        "{{ c316b8a95a73c807b4b269f84e6aa47964d8c2dae9d4bd07a68e96301e7cd7ea }}"
      ],
      "Script": "    #!{{ c316b8a95a73c807b4b269f84e6aa47964d8c2dae9d4bd07a68e96301e7cd7ea }}/bin/busybox sh\n    $busybox_store $@\n",
      "Shell": null,
      "Env": null
    }
  ]
}
```
