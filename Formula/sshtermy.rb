# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.441"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.441/sshtermy_darwin_arm64"
      sha256 "3a5f86ccbcb66961a1b9701bab04eb34d6b56f3586a1472e3009bc340a45f45e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.441/sshtermy_darwin_amd64"
      sha256 "7ce544dd13811f90953f34e0ff4b6e0cab164533894cf2cc4e41583135fa999c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.441/sshtermy_linux_arm64"
      sha256 "9a42d1da888865c949d81b76a4aa272adc28daa89d3493b8da13628666a9c6f7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.441/sshtermy_linux_amd64"
      sha256 "98fd8709e0171829c558d9e48ba2fef6569b696e4cf8378323739c7ea5a643a9"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
