# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.410"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.410/sshtermy_darwin_arm64"
      sha256 "927becb188976760b7ef9c5c15294a75b0e8d114d829bb5674d9bf5181986ed6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.410/sshtermy_darwin_amd64"
      sha256 "61d28d0c95d43bdee24e1a205c462468a5788a104d34e0e5ba77fc6481e08a38"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.410/sshtermy_linux_arm64"
      sha256 "bbdbe9e53d6fb57c265391dedde6b7654e10bec34d30e6c015ee0f7606e3ce20"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.410/sshtermy_linux_amd64"
      sha256 "7e0d5fd0f61f1c7ed33fce048beae085c794b6ddba82036cb09afd1ccdc2450b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
