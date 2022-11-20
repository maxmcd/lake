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
      script = "${foo}"
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
    busybox_tar = download_file("http://lake.com/busybox.tar.gz")
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
