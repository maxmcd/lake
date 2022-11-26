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

# Or are these docs
store "busybox_store" {
  # These are docs
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

**Json Output:**

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
      "Name": "busybox_store",
      "Inputs": [
        "{{ a4ckr4xvftvmjb4brrixkxuhevvmwt5p }}",
        "./script.sh"
      ],
      "Script": "    #!{{ a4ckr4xvftvmjb4brrixkxuhevvmwt5p }}/bin/busybox sh\n    sh ./script.sh\n",
      "Shell": null,
      "Env": {
        "FOO": "bar"
      }
    },
    {
      "Name": "busybox_store_alt",
      "Inputs": [
        "{{ a4ckr4xvftvmjb4brrixkxuhevvmwt5p }}",
        "./script.sh"
      ],
      "Script": "sh ./script.sh",
      "Shell": [
        "{{ a4ckr4xvftvmjb4brrixkxuhevvmwt5p }}/bin/busybox",
        "sh"
      ],
      "Env": null
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
        "{{ a4ckr4xvftvmjb4brrixkxuhevvmwt5p }}/bin/busybox",
        "sh"
      ],
    }
  ],
  "Targets": [
    {
      "Name": "busybox",
      "Inputs": [
        "{{ esyeusxmgnhdyfo3pmb24k7nz6qepx6c }}"
      ],
      "Script": "    #!{{ esyeusxmgnhdyfo3pmb24k7nz6qepx6c }}/bin/busybox sh\n    $busybox_store $@\n",
      "Shell": null,
      "Env": null
    }
  ]
}
```
