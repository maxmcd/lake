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
    target "empty_store" {
      inputs = []
      script = foo
    }
    store "empty_store" {
      inputs = []
      script = ""
    }
  }
}

test "weird block type" {
  err_contains = "unexpected block type"
  file "Lakefile" {
    unexpected "empty_store" {}
  }
}

test "basic functionality" {
  file "Lakefile" {
    config {
      shell = ["${busybox_tar}/bin/busybox", "sh"]
    }
    store "busybox_tar" {
      env = {
        fetch_url = "true"
        url       = "http://lake.com/busybox.tar.gz"
      }
      network = true
    }
    target "busybox" {
      inputs = [busybox_store]
      script = <<EOH
        #!${busybox_store}/bin/busybox sh
        $busybox_store $@
      EOH
    }
  }
  file "other.Lakefile" {
    ba  = "ba"
    bar = "${ba}r"
    store "busybox_store" {
      inputs = [busybox_tar, "./script.sh"]
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

test "conflicting config" {
  err_contains = "Conflicting config value"
  file "Lakefile" {
    config {
      shell = ["/bin/busybox", "sh"]
    }
  }
  file "other.Lakefile" {
    config {
      shell = ["/bin/busybox", "sh"]
    }
  }
}

test "store target circular reference" {
  err_contains = "Circular reference"
  file "Lakefile" {
    target "a" {
      inputs = [c]
      script = ""
    }
    store "b" {
      inputs = [a]
      script = ""
    }
    target "c" {
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

test "mixed argument store target circular reference" {
  err_contains = "Circular reference"
  file "Lakefile" {
    a = c
    target "b" { inputs = [a] }
    store "c" { inputs = [b] }
  }
}

test "import name conflicts with local variable" {
  err_contains = "Duplicate name"
  file "Lakefile" {
    import = ["b"]
    target "b" {}
  }
}

test "import name alias conflicts with local variable" {
  err_contains = "Duplicate name"
  file "Lakefile" {
    import = ["b", { f = "m" }]
    target "f" {}
  }
}

test "import is not at the top of the file, block is" {
  err_contains = "Invalid import location"
  file "Lakefile" {
    target "b" {}
    import = ["hi"]
  }
}

test "import is not at the top of the file, attribute is" {
  err_contains = "Invalid import location"
  file "Lakefile" {
    b      = "oh"
    import = ["hi"]
  }
}

test "no vars in import statement" {
  err_contains = "cannot contain variables"
  file "Lakefile" {
    import = ["hi", "${b}"]
    b      = "oh"
  }
}

test "import is not a list" {
  err_contains = "Import must be a list"
  file "Lakefile" {
    import = "hi"
  }
}

test "import variable conflicts across files are ok" {
  err_contains = "Import must be a list"
  file "Lakefile" {
    import = ["lake", "lock", "pond"]
    store "f" {
      imports = [lake.fish, lock.fish, pond.fish]
    }
  }
  file "foo.Lakefile" {
    store "lake" {}
    target "lock" {}
    pond = "pong"
  }

}
