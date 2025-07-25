apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
      - configmaps
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: "${encryption_secret_key}"
      - identity: {} # this fallback allows reading unencrypted secrets;
                     # for example, during initial migration
