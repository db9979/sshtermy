# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.378"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.378/sshtermy_darwin_arm64"
      sha256 "64ea915c438bc2371a338fb83c42af3037b19c0d696e9298084acef2053f0d1c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.378/sshtermy_darwin_amd64"
      sha256 "98cd7bebb7ec2491b8d4f7a0ad89785f31caa6c1fb0d8aa683c2d1cf22100071"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.378/sshtermy_linux_arm64"
      sha256 "340344c2a3e53a6ea4204a92fb169f8813f01de1f777fa9c70413a3dc9fc1e62"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.378/sshtermy_linux_amd64"
      sha256 "b3b5d07fd85e4cd6b9b8196ef5a1c15d875e76fa03c3a994aab335326a4821ac"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
