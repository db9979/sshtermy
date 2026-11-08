# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.261"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.261/sshtermy_darwin_arm64"
      sha256 "e2a0f9cdcf28aae358473f57ae6ea451df6e70437bf0b7cd35b83d32f22a92e2"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.261/sshtermy_darwin_amd64"
      sha256 "c530465315af0614af08b67b3c7265067596ee650d02958e714ff1f7e4b8c484"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.261/sshtermy_linux_arm64"
      sha256 "225ed4f84a7b50fca35a207c98d7deddf04046d6ef5b80e142f9fc4ad7fbf6ed"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.261/sshtermy_linux_amd64"
      sha256 "78c5213f7b5fbfd031cd4b11bb67343b9931cbe564488c9163c88975872fb536"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
