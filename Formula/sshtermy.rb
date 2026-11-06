# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.127"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.127/sshtermy_darwin_arm64"
      sha256 "a7d8707c29b8304a89a59fc537af41241c73a7758086e81e3598d6dc57a14b69"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.127/sshtermy_darwin_amd64"
      sha256 "d6b41181ed20db1399e20d04f38cc6eb2ff35376c7cfd37b120a4c9ded4654cd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.127/sshtermy_linux_arm64"
      sha256 "209453540f9223e62014058330df690ba3295603a339a8a2e3a06b6bee972129"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.127/sshtermy_linux_amd64"
      sha256 "21507986067e2fd1818d1824e12954c5b101a794bc1f36e63230c5a2df56ed7a"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
