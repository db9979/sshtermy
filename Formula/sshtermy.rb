# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.240"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.240/sshtermy_darwin_arm64"
      sha256 "212d1af3518b5aa6a9c2c3687d19632d58ff9b7f27678c1e8e7443c0caf4bfed"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.240/sshtermy_darwin_amd64"
      sha256 "be5dc47d052f453a891283cc51c68a1839b2abccb6be00bd32c723563580000f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.240/sshtermy_linux_arm64"
      sha256 "ba1767a705c00ddfcaa8f12786dba60ff1c6934816cd64ce3f6e25598e256953"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.240/sshtermy_linux_amd64"
      sha256 "8bc6eced284f7143ba1566eb81f243ca5d6669159fd3c650de7ce20eccc8a911"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
