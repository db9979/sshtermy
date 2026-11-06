# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.180"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.180/sshtermy_darwin_arm64"
      sha256 "1b2ecd786be16985864a29f10edbd7f6c51800cf67aa3fa05342baa02f4d7983"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.180/sshtermy_darwin_amd64"
      sha256 "3eca4cd09fa723fde77196504286b10433bf27d708b0869b0abb978c5ba4f8d4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.180/sshtermy_linux_arm64"
      sha256 "149b3ea00c208762df0dcd90652545b36ca25b3379378b216a4bc1d338cd59fe"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.180/sshtermy_linux_amd64"
      sha256 "b9d089f645e0d963c0938d670cb8b6db141d6ee303fc75e0ccb5e64da06dc378"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
