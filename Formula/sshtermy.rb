# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.157"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.157/sshtermy_darwin_arm64"
      sha256 "f90d6a6b725dafe0a879688c34d783e58a7b8f2e45558d810a4f73972ced4a50"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.157/sshtermy_darwin_amd64"
      sha256 "ca9d0dfc2ca78d0db7bd51249e048297a93d881f5da81864045bc34b1a013653"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.157/sshtermy_linux_arm64"
      sha256 "e8be658b7b29b39584884800caab7079d125efc22d144de0b0acef863e35ce59"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.157/sshtermy_linux_amd64"
      sha256 "5fd218c146fcce22a656f1fdfa619e315b23dce53e7858b87708c6c30cfecf98"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
