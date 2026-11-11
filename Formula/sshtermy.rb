# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.336"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.336/sshtermy_darwin_arm64"
      sha256 "75b4f7d71f75bf6d6f47572d0d5cfc5ca0766c6b5a19195479500f27ad28e334"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.336/sshtermy_darwin_amd64"
      sha256 "998f0defb19ef52f4466e54024710e715b263c38e8ec1f0bd68734aa30043190"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.336/sshtermy_linux_arm64"
      sha256 "4c4e245bf8745a106e554919505bc87f0bfea846e4b529b7b95077a3e2186de7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.336/sshtermy_linux_amd64"
      sha256 "ef3e973c99f3c5e7927ecf27f37a6e9a6b98ef32bc8d6644e50890ff6555dd01"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
