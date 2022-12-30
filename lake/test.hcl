test "identifier store name conflict" {
  err_contains = "Duplicate name"

  file "Lakefile" {
    empty_store = "foo"
    store "empty_store" {
      inputs = []
      script = ""
    }
  }
}

test "store and target name conflict" {
  err_contains = "Duplicate name"

  file "Lakefile" {
    command "empty_store" {
      inputs = []
      script = foo
    }
    store "empty_store" {
      inputs = []
      script = ""
    }
  }
}


test "basic out of order references" {
  err_contains = "unexpected block type"
  file "Lakefile" {
    unexpected "empty_store" {}
  }
}

test "basic functionality" {
  file "Lakefile" {

    # Remove for now?
    # defaults {
    #   shell = ["${busybox_tar}/bin/busybox", "sh"]
    # }

    store "busybox_tar" {
      env = {
        fetch_url = "true"
        url       = "http://lake.com/busybox.tar.gz"
      }
      network = true
    }

    target "busybox" {
      inputs = [busybox_store]
      shell  = ["${busybox_tar}/bin/busybox", "sh"]
      script = <<EOH
        #!${busybox_store}/bin/busybox sh
        $busybox_store $@
      EOH
    }

    ba  = "ba"
    bar = "${ba}r"
    store "busybox_store" {
      inputs = [busybox_tar, "./script.sh"]
      shell  = ["${busybox_tar}/bin/busybox", "sh"]
      env = {
        FOO = bar
      }
      script = <<EOH
        #!${busybox_tar}/bin/busybox sh
        sh ./script.sh
      EOH
    }
  }
}

# Remove for now?
# test "conflicting defaults" {
#   err_contains = "Conflicting defaults"
#   file "Lakefile" {
#     defaults {
#       shell = ["/bin/busybox", "sh"]
#     }
#     defaults {
#       shell = ["/bin/busybox", "sh"]
#     }
#   }
# }

test "store comand circular reference" {
  err_contains = "Circular reference"
  file "Lakefile" {
    command "a" {
      inputs = [c]
      script = ""
    }
    store "b" {
      inputs = [a]
      script = ""
    }
    command "c" {
      inputs = [b]
      script = ""
    }
  }
}

test "argument circular reference" {
  err_contains = "Circular reference"
  file "Lakefile" {
    a = c
    b = a
    c = b
  }
}

test "mixed argument store command circular reference" {
  err_contains = "Circular reference"
  file "Lakefile" {
    a = c
    command "b" { inputs = [a] }
    store "c" { inputs = [b] }
  }
}
