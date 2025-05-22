package migration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This currently a sample test and migration test will be added in the next PR, Otherwise it will be pretty big PR to check
// ::TODO::
var _ = Describe("Rancher Backup and Restore Migration", Label("LEVEL0", "migration"), func() {
	It("should validate the backup and restore flow", func() {
		By("Checking that the Terraform context is valid")
		Expect(tfCtx).ToNot(BeNil())
	})
})
