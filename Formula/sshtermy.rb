# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.304"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.304/sshtermy_darwin_arm64"
      sha256 "c52144b69804a952c4bfe65caafe3d33a969036c5bbdd6c8b3ac6020ad75a2df"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.304/sshtermy_darwin_amd64"
      sha256 "6a4bcd0e1129ef8e51535a3621deec3443770b0d0cad4881fd17be1a21bdbd08"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.304/sshtermy_linux_arm64"
      sha256 "b6ffd418d5a81058986aba64c578dd4c1cec9fe6a4a9d17b990aa1177056819c"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.304/sshtermy_linux_amd64"
      sha256 "247a86080c6255564bfea3680fcbe1207f7523be1dfd6b5b92ada250ce8d094f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
