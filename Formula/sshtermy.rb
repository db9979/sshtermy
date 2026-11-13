# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.386"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.386/sshtermy_darwin_arm64"
      sha256 "a6fe1fa3dbe5f2454d60f026a3925c2db4bef69d9e74938de2bc153d8de95566"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.386/sshtermy_darwin_amd64"
      sha256 "4a0d1fd0290959be7258f46b6e71732e9bb5cdc722175026f930cfc2984a4cfb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.386/sshtermy_linux_arm64"
      sha256 "bf10f181c1f2743fbc2262029e8e15110081d508cac123e6ce5fb7e70e12b7f6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.386/sshtermy_linux_amd64"
      sha256 "eb5ecf8f0592d7a6eb0598e45c680a7f68c3761bee0791d37cd986ffa5cea3ce"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
