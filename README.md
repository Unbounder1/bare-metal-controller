# bare-metal-controller

apiVersion: v1
kind: Secret
metadata:
  name: server-ssh-credentials
  namespace: power-system
type: Opaque
stringData:
  username: root
  ssh-privatekey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAA...
    ...
    -----END OPENSSH PRIVATE KEY-----
    