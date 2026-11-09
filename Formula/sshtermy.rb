# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.306"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.306/sshtermy_darwin_arm64"
      sha256 "f9e0cf6c3a6dd80e97d4682efb078e2a4dee6851b46d38bd5a3a176ae9963eb4"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.306/sshtermy_darwin_amd64"
      sha256 "68fdb67eacd781a8bc04fc354e28a44672a18d4b9de3dae3fd2fcf1b2161fef0"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.306/sshtermy_linux_arm64"
      sha256 "be5c1a4b03e7d8a2a5eb102ab2946f50778348e5575af4c9e8df884a02bc3c73"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.306/sshtermy_linux_amd64"
      sha256 "44fd6eb28baa64e35a43c907158baf1523d1cfa87ecb97b5c91e4a375edd8f74"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
