# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.171"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.171/sshtermy_darwin_arm64"
      sha256 "17a0cd7dee7dd8fc077f3bf4bc8c87ba6986529328bf9039c022707472041f91"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.171/sshtermy_darwin_amd64"
      sha256 "b4fb9b6dbbba2b0e1cf187b21b3ef3e53ceb1e70aae3abdd82574a981d25a37d"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.171/sshtermy_linux_arm64"
      sha256 "43764b48aa6631e88c56ac73f3934b2047fc80010848adbdaddbba2e42fff3cd"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.171/sshtermy_linux_amd64"
      sha256 "db018514be8e692b97b79c94027b6bee5255b5adf2122e1540f10e36e9e3e6fc"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
