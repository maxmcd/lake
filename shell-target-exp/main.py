print("I don't run!") # confirm -x argument is being used

import os
import sys
print("hi", os.environ["FOO"], sys.argv)

print(sys.stdin)
for line in sys.stdin:
    print("stdin", line)
