# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.405"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.405/sshtermy_darwin_arm64"
      sha256 "1821102d033e6ad728ac71218653b7bbd8232f8ef70c134f0b52e86cb7e317ab"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.405/sshtermy_darwin_amd64"
      sha256 "c06de26cfe68fe6112bdb3973e879cd4bfd7d4d30ce30b01aa49f0bb4de72c8c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.405/sshtermy_linux_arm64"
      sha256 "1e504bf0a2882e778572975b6a9acf6ef30e37b9a9208e629b4fa868eb4b8948"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.405/sshtermy_linux_amd64"
      sha256 "3b7236892184213c8a22b76f4e18cf9a49e2571b4f40e149a8e3d2155ad78bcb"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
